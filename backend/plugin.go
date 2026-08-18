package main

import (
	plugin "github.com/Paca-AI/plugin-sdk-go"
)

// Event topics this plugin can send email for. topicUserCreated and
// topicUserPasswordReset are mandatory — always sent (once SMTP is
// configured) with no per-user opt-out, since both are how an account gets
// (or regains) a way to set its password. The rest are optional: the admin
// can toggle each on/off, and a user may still opt out individually. All
// optional topics default to enabled (see
// migrations/0001_create_smtp_tables.sql's enabled_events default).
const (
	topicUserCreated                   = "user.created"
	topicUserPasswordReset             = "user.password_reset"
	topicNotificationAssigned          = "notification.assigned"
	topicNotificationMentioned         = "notification.mentioned"
	topicNotificationDocMentioned      = "notification.doc_mentioned"
	topicNotificationTaskDescMentioned = "notification.task_description_mentioned"
)

// optionalTopics are the events an admin can toggle on/off and a user can
// individually opt out of. topicUserCreated is deliberately excluded.
var optionalTopics = []string{
	topicNotificationAssigned,
	topicNotificationMentioned,
	topicNotificationDocMentioned,
	topicNotificationTaskDescMentioned,
}

// successEnvelope is the response shape PluginApiClient (plugin-sdk-react)
// expects from every JSON 200/201 response.
type successEnvelope struct {
	Success bool `json:"success"`
	Data    any  `json:"data"`
}

type smtpPlugin struct {
	db  *plugin.DB
	kv  *plugin.KV
	log *plugin.Logger
	cfg *plugin.Config
}

func (p *smtpPlugin) Init(ctx *plugin.Context) error {
	p.db = ctx.DB()
	p.kv = ctx.KV()
	p.log = ctx.Log()
	p.cfg = ctx.Config()

	ctx.On(topicUserCreated, p.handleEvent(topicUserCreated))
	ctx.On(topicUserPasswordReset, p.handleEvent(topicUserPasswordReset))
	ctx.On(topicNotificationAssigned, p.handleEvent(topicNotificationAssigned))
	ctx.On(topicNotificationMentioned, p.handleEvent(topicNotificationMentioned))
	ctx.On(topicNotificationDocMentioned, p.handleEvent(topicNotificationDocMentioned))
	ctx.On(topicNotificationTaskDescMentioned, p.handleEvent(topicNotificationTaskDescMentioned))

	ctx.Route("GET", "/admin/config", p.getAdminConfig)
	ctx.Route("PATCH", "/admin/config", p.updateAdminConfig)
	ctx.Route("POST", "/admin/test-email", p.sendTestEmail)
	ctx.Route("GET", "/me/preferences", p.getMyPreferences)
	ctx.Route("PATCH", "/me/preferences", p.updateMyPreferences)

	p.log.Info("com.paca.smtp initialized")
	return nil
}

func (p *smtpPlugin) Shutdown() {
	p.log.Info("com.paca.smtp shutdown")
}
