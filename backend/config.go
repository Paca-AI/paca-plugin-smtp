package main

import (
	"encoding/json"
	"strings"

	plugin "github.com/Paca-AI/plugin-sdk-go"
)

// smtpConfig is the in-memory shape of the smtp_config singleton row.
// Password is deliberately absent — it's write-only, see loadConfig.
type smtpConfig struct {
	Host          string
	Port          int
	Username      string
	FromAddress   string
	FromName      string
	UseTLS        bool
	EnabledEvents []string
	HasPassword   bool
}

// loadConfig reads the smtp_config singleton row, returning the config
// (never including the decrypted password) and the raw encrypted password
// separately, for callers (sendEmail paths) that need to decrypt it.
func (p *smtpPlugin) loadConfig() (*smtpConfig, string, error) {
	result, err := p.db.Query(`SELECT host, port, username, password_enc, from_address, from_name, use_tls, enabled_events FROM smtp_config WHERE id = $1`, true)
	if err != nil {
		return nil, "", err
	}
	if len(result.Rows) == 0 {
		// Should not happen in practice (the migration inserts the singleton
		// row unconditionally) but fall back to the same "everything on"
		// default as that row for consistency if it ever does.
		return &smtpConfig{Port: 587, UseTLS: true, EnabledEvents: append([]string(nil), optionalTopics...)}, "", nil
	}

	row := result.Rows[0]
	var enabled []string
	_ = json.Unmarshal([]byte(toString(row[7])), &enabled)
	if enabled == nil {
		enabled = []string{}
	}
	passwordEnc := toString(row[3])

	cfg := &smtpConfig{
		Host:          toString(row[0]),
		Port:          toInt(row[1]),
		Username:      toString(row[2]),
		FromAddress:   toString(row[4]),
		FromName:      toString(row[5]),
		UseTLS:        toBool(row[6]),
		EnabledEvents: enabled,
		HasPassword:   passwordEnc != "",
	}
	return cfg, passwordEnc, nil
}

// isConfigured reports whether cfg has enough set to actually send mail.
func (cfg *smtpConfig) isConfigured() bool {
	return cfg.Host != "" && cfg.FromAddress != ""
}

type adminConfigResponse struct {
	Host          string   `json:"host"`
	Port          int      `json:"port"`
	Username      string   `json:"username"`
	FromAddress   string   `json:"from_address"`
	FromName      string   `json:"from_name"`
	UseTLS        bool     `json:"use_tls"`
	EnabledEvents []string `json:"enabled_events"`
	HasPassword   bool     `json:"has_password"`
}

func toAdminConfigResponse(cfg *smtpConfig) adminConfigResponse {
	return adminConfigResponse{
		Host:          cfg.Host,
		Port:          cfg.Port,
		Username:      cfg.Username,
		FromAddress:   cfg.FromAddress,
		FromName:      cfg.FromName,
		UseTLS:        cfg.UseTLS,
		EnabledEvents: cfg.EnabledEvents,
		HasPassword:   cfg.HasPassword,
	}
}

// getAdminConfig handles GET /admin/config.
func (p *smtpPlugin) getAdminConfig(_ *plugin.Request, res *plugin.Response) {
	cfg, _, err := p.loadConfig()
	if err != nil {
		p.log.Error("getAdminConfig: " + err.Error())
		res.Error(500, "failed to load config")
		return
	}
	res.JSON(200, successEnvelope{Success: true, Data: toAdminConfigResponse(cfg)})
}

type updateConfigBody struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	// Password is optional on update: an empty value keeps the existing
	// stored password unchanged (it's never round-tripped back to the UI in
	// plaintext, so "leave blank to keep the current one" is the only way
	// to update everything else without also having to re-enter it).
	Password      string   `json:"password"`
	FromAddress   string   `json:"from_address"`
	FromName      string   `json:"from_name"`
	UseTLS        bool     `json:"use_tls"`
	EnabledEvents []string `json:"enabled_events"`
}

// updateAdminConfig handles PATCH /admin/config.
func (p *smtpPlugin) updateAdminConfig(req *plugin.Request, res *plugin.Response) {
	body, err := plugin.JSONBody[updateConfigBody](req)
	if err != nil {
		res.Error(400, "invalid JSON body")
		return
	}
	if strings.TrimSpace(body.Host) == "" || body.Port <= 0 {
		res.Error(400, "host and port are required")
		return
	}
	if strings.TrimSpace(body.FromAddress) == "" {
		res.Error(400, "from_address is required")
		return
	}

	_, existingEnc, err := p.loadConfig()
	if err != nil {
		p.log.Error("updateAdminConfig: load existing: " + err.Error())
		res.Error(500, "failed to load existing config")
		return
	}
	passwordEnc := existingEnc
	if body.Password != "" {
		enc, err := p.encrypt(body.Password)
		if err != nil {
			p.log.Error("updateAdminConfig: encrypt: " + err.Error())
			res.Error(500, "failed to secure password — is ENCRYPTION_KEY configured?")
			return
		}
		passwordEnc = enc
	}

	events := sanitizeEventNames(body.EnabledEvents)
	eventsJSON, _ := json.Marshal(events)

	if _, err := p.db.Exec(
		`UPDATE smtp_config SET host = $1, port = $2, username = $3, password_enc = $4,
		 from_address = $5, from_name = $6, use_tls = $7, enabled_events = $8, updated_at = $9
		 WHERE id = $10`,
		body.Host, body.Port, body.Username, passwordEnc,
		body.FromAddress, body.FromName, body.UseTLS, string(eventsJSON), nowStr(), true,
	); err != nil {
		p.log.Error("updateAdminConfig: " + err.Error())
		res.Error(500, "failed to save config")
		return
	}

	cfg, _, err := p.loadConfig()
	if err != nil {
		res.Error(500, "failed to reload config")
		return
	}
	res.JSON(200, successEnvelope{Success: true, Data: toAdminConfigResponse(cfg)})
}

// sanitizeEventNames keeps only recognised optional topics from in,
// deduplicated, in a stable order.
func sanitizeEventNames(in []string) []string {
	allowed := make(map[string]bool, len(optionalTopics))
	for _, t := range optionalTopics {
		allowed[t] = true
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, e := range in {
		if allowed[e] && !seen[e] {
			out = append(out, e)
			seen[e] = true
		}
	}
	return out
}

type sendTestEmailBody struct {
	To string `json:"to"`
}

// sendTestEmail handles POST /admin/test-email — sends a sample email using
// the currently-saved config to let the admin verify it actually works.
func (p *smtpPlugin) sendTestEmail(req *plugin.Request, res *plugin.Response) {
	cfg, passwordEnc, err := p.loadConfig()
	if err != nil {
		res.Error(500, "failed to load config")
		return
	}
	if !cfg.isConfigured() {
		res.Error(400, "configure and save your SMTP settings first")
		return
	}

	body, err := plugin.JSONBody[sendTestEmailBody](req)
	if err != nil {
		res.Error(400, "invalid JSON body")
		return
	}
	to := strings.TrimSpace(body.To)
	if to == "" {
		res.Error(400, "to is required")
		return
	}

	password, err := p.decrypt(passwordEnc)
	if err != nil {
		p.log.Error("sendTestEmail: decrypt: " + err.Error())
		res.Error(500, "failed to read stored password")
		return
	}

	branding, _ := plugin.GetBranding()
	subject, html, text := renderTestEmail(branding)

	if err := p.sendEmail(sendEmailInput{
		Host: cfg.Host, Port: cfg.Port, Username: cfg.Username, Password: password,
		UseTLS: cfg.UseTLS, From: cfg.FromAddress, FromName: cfg.FromName,
		To: to, Subject: subject, HTMLBody: html, TextBody: text,
	}); err != nil {
		p.log.Error("sendTestEmail: send: " + err.Error())
		res.Error(502, "failed to send: "+err.Error())
		return
	}
	res.NoContent()
}
