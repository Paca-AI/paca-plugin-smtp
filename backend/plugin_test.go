package main

import (
	"encoding/json"
	"strings"
	"testing"

	plugin "github.com/Paca-AI/plugin-sdk-go"
	"github.com/Paca-AI/plugin-sdk-go/plugintest"
)

const testEncryptionKey = "3fead24473a9a7bf262857db0b4de648c86de5a29b3b3bb5bfb46875ede0d7de"

// smtpConfigColumns is the seed column order for smtp_config in tests. It
// mirrors loadConfig's SELECT list (host, port, username, password_enc,
// from_address, from_name, use_tls, enabled_events) — not the migration's
// physical column order — with "id" and "updated_at" appended so the WHERE
// id = $1 and SET updated_at = $N clauses have columns to resolve against.
// plugintest's InMemoryDB returns whole rows rather than projecting the
// requested SELECT columns, so a seeded row must be laid out in the exact
// order the plugin code positionally decodes it in.
var smtpConfigColumns = []string{
	"host", "port", "username", "password_enc",
	"from_address", "from_name", "use_tls", "enabled_events", "id", "updated_at",
}

// preferencesColumns mirrors loadUserPreferences' SELECT disabled_events
// list, with "user_id" appended for the WHERE clause — see
// smtpConfigColumns' doc comment for why the order matters.
var preferencesColumns = []string{"disabled_events", "user_id"}

func setupPlugin(t *testing.T) *plugintest.Context {
	t.Helper()
	tc := plugintest.NewContext(t)
	tc.DB.SeedRows("smtp_config", smtpConfigColumns, nil)
	tc.DB.SeedRows("user_email_preferences", preferencesColumns, nil)

	var p smtpPlugin
	if err := p.Init(tc.PluginContext()); err != nil {
		t.Fatalf("plugin init failed: %v", err)
	}
	return tc
}

// seedSmtpConfig replaces the smtp_config singleton row with the given
// values, laid out per smtpConfigColumns.
func seedSmtpConfig(tc *plugintest.Context, host string, port int, username, passwordEnc, fromAddress, fromName string, useTLS bool, enabledEventsJSON string) {
	tc.DB.SeedRows("smtp_config", smtpConfigColumns, [][]any{
		{host, port, username, passwordEnc, fromAddress, fromName, useTLS, enabledEventsJSON, true, nowStr()},
	})
}

// seedUserPreferences replaces user_email_preferences with a single row for
// userID, laid out per preferencesColumns.
func seedUserPreferences(tc *plugintest.Context, userID, disabledEventsJSON string) {
	tc.DB.SeedRows("user_email_preferences", preferencesColumns, [][]any{
		{disabledEventsJSON, userID},
	})
}

func adminReq() plugintest.Request {
	return plugintest.Request{
		Caller: plugin.CallerIdentity{CallerID: "member-1", CallerRole: "GLOBAL_ADMIN"},
	}
}

func userReq(userID string) plugintest.Request {
	return plugintest.Request{
		Caller: plugin.CallerIdentity{CallerID: "member-1", CallerRole: "PROJECT_MEMBER", UserID: userID},
	}
}

func decodeEnvelope[T any](t *testing.T, res *plugin.Response) T {
	t.Helper()
	var env struct {
		Success bool `json:"success"`
		Data    T    `json:"data"`
	}
	if err := json.Unmarshal(res.Body, &env); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, res.BodyString())
	}
	if !env.Success {
		t.Fatalf("expected success envelope, got %s", res.BodyString())
	}
	return env.Data
}

// ── GET /admin/config ────────────────────────────────────────────────────────

func TestGetAdminConfig_DefaultsWhenRowMissing(t *testing.T) {
	tc := setupPlugin(t)

	res := tc.Call("GET", "/admin/config", adminReq())
	if res.StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", res.StatusCode, res.BodyString())
	}

	cfg := decodeEnvelope[adminConfigResponse](t, res)
	if cfg.Port != 587 || !cfg.UseTLS || cfg.HasPassword {
		t.Fatalf("unexpected default config: %+v", cfg)
	}
	if len(cfg.EnabledEvents) != len(optionalTopics) {
		t.Fatalf("expected all optional topics enabled by default, got %v", cfg.EnabledEvents)
	}
}

func TestGetAdminConfig_ReturnsStoredValues(t *testing.T) {
	tc := setupPlugin(t)
	seedSmtpConfig(tc, "smtp.example.com", 465, "svc-account", "enc-pw",
		"noreply@example.com", "Paca", true, `["notification.assigned"]`)

	res := tc.Call("GET", "/admin/config", adminReq())
	if res.StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", res.StatusCode, res.BodyString())
	}

	cfg := decodeEnvelope[adminConfigResponse](t, res)
	if cfg.Host != "smtp.example.com" || cfg.Port != 465 || cfg.Username != "svc-account" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if cfg.FromAddress != "noreply@example.com" || cfg.FromName != "Paca" || !cfg.UseTLS {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if !cfg.HasPassword {
		t.Fatal("expected has_password=true when password_enc is set")
	}
	if len(cfg.EnabledEvents) != 1 || cfg.EnabledEvents[0] != "notification.assigned" {
		t.Fatalf("unexpected enabled_events: %v", cfg.EnabledEvents)
	}
}

// ── PATCH /admin/config ──────────────────────────────────────────────────────

func TestUpdateAdminConfig_Validation(t *testing.T) {
	cases := []struct {
		name string
		body map[string]any
	}{
		{"missing host", map[string]any{"host": "", "port": 587, "from_address": "a@b.com"}},
		{"zero port", map[string]any{"host": "smtp.example.com", "port": 0, "from_address": "a@b.com"}},
		{"negative port", map[string]any{"host": "smtp.example.com", "port": -1, "from_address": "a@b.com"}},
		{"missing from_address", map[string]any{"host": "smtp.example.com", "port": 587, "from_address": ""}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tc := setupPlugin(t)
			res := tc.Call("PATCH", "/admin/config", adminReq().WithJSONBody(c.body))
			if res.StatusCode != 400 {
				t.Fatalf("expected 400, got %d: %s", res.StatusCode, res.BodyString())
			}
		})
	}
}

func TestUpdateAdminConfig_InvalidJSON(t *testing.T) {
	tc := setupPlugin(t)
	req := adminReq()
	req.Body = []byte("not json")
	req.Headers = map[string]string{"content-type": "application/json"}

	res := tc.Call("PATCH", "/admin/config", req)
	if res.StatusCode != 400 {
		t.Fatalf("expected 400, got %d: %s", res.StatusCode, res.BodyString())
	}
}

func TestUpdateAdminConfig_PasswordWithoutEncryptionKeyFails(t *testing.T) {
	tc := setupPlugin(t)

	res := tc.Call("PATCH", "/admin/config", adminReq().WithJSONBody(map[string]any{
		"host": "smtp.example.com", "port": 587,
		"from_address": "noreply@example.com",
		"password":     "s3cr3t",
	}))
	if res.StatusCode != 500 {
		t.Fatalf("expected 500, got %d: %s", res.StatusCode, res.BodyString())
	}
}

func TestUpdateAdminConfig_PersistsChanges(t *testing.T) {
	tc := setupPlugin(t)
	seedSmtpConfig(tc, "", 587, "", "", "", "", true, `[]`)
	tc.Config.Set("ENCRYPTION_KEY", testEncryptionKey)

	res := tc.Call("PATCH", "/admin/config", adminReq().WithJSONBody(map[string]any{
		"host": "smtp.example.com", "port": 2525, "username": "svc-account",
		"password": "s3cr3t", "from_address": "noreply@example.com", "from_name": "Paca",
		"use_tls":        false,
		"enabled_events": []string{"notification.assigned", "notification.assigned", "bogus.topic"},
	}))
	if res.StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", res.StatusCode, res.BodyString())
	}
	cfg := decodeEnvelope[adminConfigResponse](t, res)
	if cfg.Host != "smtp.example.com" || cfg.Port != 2525 || cfg.Username != "svc-account" {
		t.Fatalf("unexpected persisted config: %+v", cfg)
	}
	if cfg.UseTLS {
		t.Fatal("expected use_tls to be persisted as false")
	}
	if !cfg.HasPassword {
		t.Fatal("expected has_password=true after setting a password")
	}
	// Unknown topics are dropped and duplicates de-duplicated.
	if len(cfg.EnabledEvents) != 1 || cfg.EnabledEvents[0] != "notification.assigned" {
		t.Fatalf("expected sanitised enabled_events, got %v", cfg.EnabledEvents)
	}

	// GET reflects the same persisted state on a fresh request.
	get := tc.Call("GET", "/admin/config", adminReq())
	if get.StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", get.StatusCode, get.BodyString())
	}
}

func TestUpdateAdminConfig_BlankPasswordKeepsExisting(t *testing.T) {
	tc := setupPlugin(t)
	seedSmtpConfig(tc, "smtp.example.com", 587, "svc-account", "existing-enc-pw",
		"noreply@example.com", "Paca", true, `[]`)

	res := tc.Call("PATCH", "/admin/config", adminReq().WithJSONBody(map[string]any{
		"host": "smtp.example.com", "port": 587, "username": "svc-account",
		"password": "", "from_address": "noreply@example.com", "from_name": "Paca",
		"use_tls": true,
	}))
	if res.StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", res.StatusCode, res.BodyString())
	}
	cfg := decodeEnvelope[adminConfigResponse](t, res)
	if !cfg.HasPassword {
		t.Fatal("expected the previously-stored password to survive a blank password field")
	}
}

// ── POST /admin/test-email ───────────────────────────────────────────────────

func TestSendTestEmail_RequiresConfiguredSMTP(t *testing.T) {
	tc := setupPlugin(t)
	res := tc.Call("POST", "/admin/test-email", adminReq().WithJSONBody(map[string]any{"to": "a@b.com"}))
	if res.StatusCode != 400 {
		t.Fatalf("expected 400, got %d: %s", res.StatusCode, res.BodyString())
	}
}

func TestSendTestEmail_RequiresRecipient(t *testing.T) {
	tc := setupPlugin(t)
	seedSmtpConfig(tc, "smtp.example.com", 587, "", "", "noreply@example.com", "Paca", true, `[]`)

	res := tc.Call("POST", "/admin/test-email", adminReq().WithJSONBody(map[string]any{"to": "  "}))
	if res.StatusCode != 400 {
		t.Fatalf("expected 400, got %d: %s", res.StatusCode, res.BodyString())
	}
}

// TestSendTestEmail_FailsNativeOutsideWASM confirms the plugin reaches the
// host send_email boundary — actual SMTP dispatch only exists inside the
// WASM build (see mail_native.go), so the native test binary always sees the
// "only available when running as a WASM plugin" error here.
func TestSendTestEmail_FailsNativeOutsideWASM(t *testing.T) {
	tc := setupPlugin(t)
	seedSmtpConfig(tc, "smtp.example.com", 587, "", "", "noreply@example.com", "Paca", true, `[]`)

	res := tc.Call("POST", "/admin/test-email", adminReq().WithJSONBody(map[string]any{"to": "a@b.com"}))
	if res.StatusCode != 502 {
		t.Fatalf("expected 502, got %d: %s", res.StatusCode, res.BodyString())
	}
	if !strings.Contains(res.BodyString(), "only available when running as a WASM plugin") {
		t.Fatalf("unexpected error body: %s", res.BodyString())
	}
}

// ── /me/preferences ───────────────────────────────────────────────────────────

func TestMyPreferences_RequiresAuthentication(t *testing.T) {
	tc := setupPlugin(t)

	get := tc.Call("GET", "/me/preferences", plugintest.Request{})
	if get.StatusCode != 401 {
		t.Fatalf("expected 401, got %d: %s", get.StatusCode, get.BodyString())
	}

	patch := tc.Call("PATCH", "/me/preferences", plugintest.Request{}.WithJSONBody(map[string]any{}))
	if patch.StatusCode != 401 {
		t.Fatalf("expected 401, got %d: %s", patch.StatusCode, patch.BodyString())
	}
}

func TestGetMyPreferences_DefaultsWhenNoRow(t *testing.T) {
	tc := setupPlugin(t)
	seedSmtpConfig(tc, "smtp.example.com", 587, "", "", "noreply@example.com", "Paca", true,
		`["notification.assigned","notification.mentioned"]`)

	res := tc.Call("GET", "/me/preferences", userReq("user-1"))
	if res.StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", res.StatusCode, res.BodyString())
	}
	prefs := decodeEnvelope[preferencesResponse](t, res)
	if len(prefs.DisabledEvents) != 0 {
		t.Fatalf("expected no disabled events for a user with no saved row, got %v", prefs.DisabledEvents)
	}
	if len(prefs.AvailableEvents) != 2 {
		t.Fatalf("expected available_events to mirror admin config, got %v", prefs.AvailableEvents)
	}
}

func TestGetMyPreferences_ReturnsStoredDisabledEvents(t *testing.T) {
	tc := setupPlugin(t)
	seedSmtpConfig(tc, "smtp.example.com", 587, "", "", "noreply@example.com", "Paca", true,
		`["notification.assigned","notification.mentioned"]`)
	seedUserPreferences(tc, "user-1", `["notification.mentioned"]`)

	res := tc.Call("GET", "/me/preferences", userReq("user-1"))
	if res.StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", res.StatusCode, res.BodyString())
	}
	prefs := decodeEnvelope[preferencesResponse](t, res)
	if len(prefs.DisabledEvents) != 1 || prefs.DisabledEvents[0] != "notification.mentioned" {
		t.Fatalf("unexpected disabled_events: %v", prefs.DisabledEvents)
	}
}

func TestUpdateMyPreferences_InvalidJSON(t *testing.T) {
	tc := setupPlugin(t)
	req := userReq("user-1")
	req.Body = []byte("not json")
	req.Headers = map[string]string{"content-type": "application/json"}

	res := tc.Call("PATCH", "/me/preferences", req)
	if res.StatusCode != 400 {
		t.Fatalf("expected 400, got %d: %s", res.StatusCode, res.BodyString())
	}
}

// ── Event handling ───────────────────────────────────────────────────────────

func TestHandleEvent_NoOpWhenSMTPNotConfigured(t *testing.T) {
	tc := setupPlugin(t)
	// smtp_config row present (matching the state right after the plugin's
	// migration runs) but host/from_address are still blank.
	seedSmtpConfig(tc, "", 587, "", "", "", "", true, `[]`)

	before := len(tc.Log.Entries())
	payload, _ := json.Marshal(passwordSetEventPayload{UserID: "u1", Email: "u1@example.com"})
	if ok := plugin.DispatchEvent(tc.PluginContext(), topicUserCreated, payload); !ok {
		t.Fatal("expected user.created handler to be registered")
	}
	if len(tc.Log.Entries()) != before {
		t.Fatalf("expected no new log entries when SMTP isn't configured, got %v", tc.Log.Entries()[before:])
	}
}

func TestHandleEvent_UserCreated_ReachesTokenIssuance(t *testing.T) {
	tc := setupPlugin(t)
	seedSmtpConfig(tc, "smtp.example.com", 587, "", "", "noreply@example.com", "Paca", true, `[]`)
	tc.Config.Set("PUBLIC_URL", "https://paca.example.com")

	payload, _ := json.Marshal(passwordSetEventPayload{UserID: "u1", FullName: "New User", Email: "u1@example.com"})
	if ok := plugin.DispatchEvent(tc.PluginContext(), topicUserCreated, payload); !ok {
		t.Fatal("expected user.created handler to be registered")
	}
	// Real token issuance is a WASM-host-only capability (see
	// password_token_native.go); outside WASM the handler logs and stops
	// there instead of sending anything.
	if !tc.Log.HasMessage("issue password set token") {
		t.Fatalf("expected a token-issuance log entry, got %+v", tc.Log.Entries())
	}
}

func TestHandleEvent_Notification_SkipsWhenRecipientOptedOut(t *testing.T) {
	tc := setupPlugin(t)
	seedSmtpConfig(tc, "smtp.example.com", 587, "", "", "noreply@example.com", "Paca", true,
		`["notification.assigned"]`)
	seedUserPreferences(tc, "u2", `["notification.assigned"]`)

	before := len(tc.Log.Entries())
	payload, _ := json.Marshal(notificationPayload{
		RecipientUserID: "u2", RecipientEmail: "u2@example.com", RecipientName: "U2",
		ActorName: "Alice", LinkURL: "https://paca.example.com/tasks/1",
	})
	if ok := plugin.DispatchEvent(tc.PluginContext(), topicNotificationAssigned, payload); !ok {
		t.Fatal("expected notification.assigned handler to be registered")
	}
	if len(tc.Log.Entries()) != before {
		t.Fatalf("expected an opted-out recipient to produce no log entries, got %v", tc.Log.Entries()[before:])
	}
}

func TestHandleEvent_Notification_ReachesSendBoundary(t *testing.T) {
	tc := setupPlugin(t)
	seedSmtpConfig(tc, "smtp.example.com", 587, "", "", "noreply@example.com", "Paca", true,
		`["notification.assigned"]`)

	payload, _ := json.Marshal(notificationPayload{
		RecipientUserID: "u2", RecipientEmail: "u2@example.com", RecipientName: "U2",
		ActorName: "Alice", LinkURL: "https://paca.example.com/tasks/1",
	})
	if ok := plugin.DispatchEvent(tc.PluginContext(), topicNotificationAssigned, payload); !ok {
		t.Fatal("expected notification.assigned handler to be registered")
	}
	if !tc.Log.HasMessage("only available when running as a WASM plugin") {
		t.Fatalf("expected the handler to reach the native send_email stub, got %+v", tc.Log.Entries())
	}
}

func TestHandleEvent_Notification_NoOpWhenRecipientEmailMissing(t *testing.T) {
	tc := setupPlugin(t)
	seedSmtpConfig(tc, "smtp.example.com", 587, "", "", "noreply@example.com", "Paca", true,
		`["notification.mentioned"]`)

	before := len(tc.Log.Entries())
	payload, _ := json.Marshal(notificationPayload{RecipientUserID: "u3", ActorName: "Alice"})
	if ok := plugin.DispatchEvent(tc.PluginContext(), topicNotificationMentioned, payload); !ok {
		t.Fatal("expected notification.mentioned handler to be registered")
	}
	if len(tc.Log.Entries()) != before {
		t.Fatalf("expected no log entries for a payload with no recipient email, got %v", tc.Log.Entries()[before:])
	}
}

func TestHandleEvent_UnknownTopicIgnored(t *testing.T) {
	tc := setupPlugin(t)
	if ok := plugin.DispatchEvent(tc.PluginContext(), "some.unregistered.topic", []byte(`{}`)); ok {
		t.Fatal("expected no handler registered for an unrelated topic")
	}
}

// ── Pure helpers ──────────────────────────────────────────────────────────────

func TestSanitizeEventNames(t *testing.T) {
	got := sanitizeEventNames([]string{
		"notification.assigned", "notification.assigned", "bogus.topic", "notification.mentioned",
	})
	want := []string{"notification.assigned", "notification.mentioned"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestContainsString(t *testing.T) {
	if !containsString([]string{"a", "b"}, "b") {
		t.Fatal("expected true")
	}
	if containsString([]string{"a", "b"}, "c") {
		t.Fatal("expected false")
	}
	if containsString(nil, "a") {
		t.Fatal("expected false for nil list")
	}
}

func TestToStringToIntToBool(t *testing.T) {
	if toString(nil) != "" {
		t.Fatal("toString(nil) should be empty")
	}
	if toString("x") != "x" {
		t.Fatal("toString should pass through strings")
	}
	if toString(42) != "42" {
		t.Fatalf("toString(42) = %q", toString(42))
	}

	if toInt(float64(3)) != 3 {
		t.Fatal("toInt should handle float64 (JSON bridge numeric type)")
	}
	if toInt("7") != 7 {
		t.Fatal("toInt should parse numeric strings")
	}
	if toInt("nope") != 0 {
		t.Fatal("toInt should default to 0 for unparseable strings")
	}

	if !toBool(true) {
		t.Fatal("toBool(true) should be true")
	}
	if !toBool("t") || !toBool("1") {
		t.Fatal("toBool should recognise common truthy string forms")
	}
	if toBool("false") {
		t.Fatal("toBool(\"false\") should be false")
	}
}

func TestEncryptDecryptAES_RoundTrip(t *testing.T) {
	cipherHex, err := encryptAES("hunter2", testEncryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := decryptAES(cipherHex, testEncryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	if plain != "hunter2" {
		t.Fatalf("expected round trip to recover the plaintext, got %q", plain)
	}
}

func TestEncryptAES_RejectsWrongKeyLength(t *testing.T) {
	if _, err := encryptAES("hunter2", "deadbeef"); err == nil {
		t.Fatal("expected an error for a key that isn't 32 bytes")
	}
}

func TestPluginEncryptDecrypt(t *testing.T) {
	tc := setupPlugin(t)
	var p smtpPlugin
	_ = p.Init(tc.PluginContext())

	if enc, err := p.encrypt(""); err != nil || enc != "" {
		t.Fatalf("encrypting an empty password should no-op, got (%q, %v)", enc, err)
	}
	if _, err := p.encrypt("hunter2"); err == nil {
		t.Fatal("expected an error when ENCRYPTION_KEY isn't configured")
	}

	tc.Config.Set("ENCRYPTION_KEY", testEncryptionKey)
	enc, err := p.encrypt("hunter2")
	if err != nil {
		t.Fatal(err)
	}
	dec, err := p.decrypt(enc)
	if err != nil {
		t.Fatal(err)
	}
	if dec != "hunter2" {
		t.Fatalf("expected round trip, got %q", dec)
	}
}

func TestRenderTestEmail(t *testing.T) {
	subject, html, text := renderTestEmail(nil)
	if !strings.Contains(subject, "test email") {
		t.Fatalf("unexpected subject: %q", subject)
	}
	if !strings.Contains(html, "SMTP configuration works") {
		t.Fatal("expected the HTML body to describe a successful test")
	}
	if !strings.Contains(text, "SMTP configuration works") {
		t.Fatal("expected the text body to describe a successful test")
	}
}

func TestRenderPasswordSetEmail(t *testing.T) {
	data := passwordSetEventPayload{UserID: "u1", FullName: "Ada Lovelace", Email: "ada@example.com"}
	subject, html, text := renderPasswordSetEmail(nil, topicUserCreated, data, "https://paca.example.com/set-password?token=abc")

	if !strings.Contains(subject, "set your password") {
		t.Fatalf("unexpected subject: %q", subject)
	}
	if !strings.Contains(html, "https://paca.example.com/set-password?token=abc") {
		t.Fatal("expected the invite URL to appear in the HTML body")
	}
	if !strings.Contains(text, "https://paca.example.com/set-password?token=abc") {
		t.Fatal("expected the invite URL to appear in the text body")
	}

	resetSubject, _, _ := renderPasswordSetEmail(nil, topicUserPasswordReset, data, "https://paca.example.com/set-password?token=xyz")
	if !strings.Contains(resetSubject, "password was reset") {
		t.Fatalf("unexpected reset subject: %q", resetSubject)
	}
}

func TestRenderNotificationEmail(t *testing.T) {
	data := notificationPayload{ActorName: "Alice", LinkURL: "https://paca.example.com/tasks/1"}

	for _, topic := range []string{
		topicNotificationAssigned, topicNotificationMentioned,
		topicNotificationDocMentioned, topicNotificationTaskDescMentioned,
	} {
		subject, html, text := renderNotificationEmail(nil, topic, data)
		if subject == "" {
			t.Fatalf("expected a non-empty subject for topic %q", topic)
		}
		if !strings.Contains(html, data.LinkURL) || !strings.Contains(text, data.LinkURL) {
			t.Fatalf("expected the link URL to appear in both bodies for topic %q", topic)
		}
	}
}

func TestBrandFrom_FallsBackToDefaults(t *testing.T) {
	name, logoURL, primaryColor := brandFrom(nil)
	if name != defaultBrandName || logoURL != defaultLogoURL || primaryColor != defaultPrimaryColor {
		t.Fatalf("expected defaults, got (%q, %q, %q)", name, logoURL, primaryColor)
	}

	name, _, primaryColor = brandFrom(&plugin.BrandingInfo{BrandName: "Acme", PrimaryColorLight: "#123456"})
	if name != "Acme" || primaryColor != "#123456" {
		t.Fatalf("expected custom branding to override defaults, got (%q, %q)", name, primaryColor)
	}
}
