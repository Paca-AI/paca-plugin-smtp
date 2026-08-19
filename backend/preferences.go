package main

import (
	"encoding/json"

	plugin "github.com/Paca-AI/plugin-sdk-go"
)

// loadUserPreferences returns the topics userID has individually opted out
// of. A user with no row yet has opted out of nothing.
func (p *smtpPlugin) loadUserPreferences(userID string) ([]string, error) {
	result, err := p.db.Query(`SELECT disabled_events FROM user_email_preferences WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	if len(result.Rows) == 0 {
		return []string{}, nil
	}
	var disabled []string
	_ = json.Unmarshal([]byte(toString(result.Rows[0][0])), &disabled)
	if disabled == nil {
		disabled = []string{}
	}
	return disabled, nil
}

type preferencesResponse struct {
	// AvailableEvents are the optional events the admin has enabled — the
	// only ones meaningful to show a per-user toggle for. topicUserCreated
	// never appears here; it has no per-user opt-out (see plugin.go).
	AvailableEvents []string `json:"available_events"`
	DisabledEvents  []string `json:"disabled_events"`
}

// getMyPreferences handles GET /me/preferences.
func (p *smtpPlugin) getMyPreferences(req *plugin.Request, res *plugin.Response) {
	userID := req.Caller.UserID
	if userID == "" {
		res.Error(401, "unauthenticated")
		return
	}

	cfg, _, err := p.loadConfig()
	if err != nil {
		res.Error(500, "failed to load config")
		return
	}
	disabled, err := p.loadUserPreferences(userID)
	if err != nil {
		res.Error(500, "failed to load preferences")
		return
	}

	res.JSON(200, successEnvelope{Success: true, Data: preferencesResponse{
		AvailableEvents: cfg.EnabledEvents,
		DisabledEvents:  disabled,
	}})
}

type updatePreferencesBody struct {
	DisabledEvents []string `json:"disabled_events"`
}

// updateMyPreferences handles PATCH /me/preferences.
func (p *smtpPlugin) updateMyPreferences(req *plugin.Request, res *plugin.Response) {
	userID := req.Caller.UserID
	if userID == "" {
		res.Error(401, "unauthenticated")
		return
	}

	body, err := plugin.JSONBody[updatePreferencesBody](req)
	if err != nil {
		res.Error(400, "invalid JSON body")
		return
	}
	disabled := sanitizeEventNames(body.DisabledEvents)
	disabledJSON, _ := json.Marshal(disabled)

	if _, err := p.db.Exec(
		`INSERT INTO user_email_preferences (user_id, disabled_events, updated_at)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (user_id) DO UPDATE SET disabled_events = EXCLUDED.disabled_events, updated_at = EXCLUDED.updated_at`,
		userID, string(disabledJSON), nowStr(),
	); err != nil {
		p.log.Error("updateMyPreferences: " + err.Error())
		res.Error(500, "failed to save preferences")
		return
	}
	res.NoContent()
}
