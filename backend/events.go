package main

import (
	"fmt"
	"strings"

	plugin "github.com/Paca-AI/plugin-sdk-go"
)

// passwordSetEventPayload mirrors the user.created and user.password_reset
// event payloads published by services/api's user.Service (see
// publishUserIdentityEvent) — same shape for both topics, distinguished by
// the topic string itself, since both events leave the account in the same
// must-change-password state and trigger the same "set your password"
// email from this plugin. Deliberately no token/invite-link field: a
// password-set token is a bearer credential, never included in an event
// payload fanned out to every subscriber. This plugin instead calls
// paca.password_set_token_issue (see issuePasswordSetToken) on demand, once
// it has decided to act on the event.
type passwordSetEventPayload struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	FullName string `json:"full_name"`
	Email    string `json:"email"`
}

// notificationPayload mirrors the notification.{assigned,mentioned,
// doc_mentioned,task_description_mentioned} event payloads published by
// services/api's notification.Svc (see publishNotificationEvent) — the same
// shape for all four topics, distinguished by the topic string itself.
// recipient_email may be empty (the recipient has no email on file); this
// plugin treats that as "nothing to do" and no-ops, per the event-handling
// switch below.
type notificationPayload struct {
	RecipientUserID string `json:"recipient_user_id"`
	RecipientEmail  string `json:"recipient_email"`
	RecipientName   string `json:"recipient_name"`
	ActorName       string `json:"actor_name"`
	LinkURL         string `json:"link_url"`
	// EntityTitle is the title of the task (assigned/mentioned/
	// task_description_mentioned) or document (doc_mentioned) the
	// notification is about. May be empty — an older core that predates the
	// field, or one that couldn't resolve the title — in which case the copy
	// below falls back to its original title-less phrasing.
	EntityTitle string `json:"entity_title"`
}

// handleEvent returns an EventHandler that sends the appropriate email for
// topic, gated by whether SMTP is configured, whether the admin has enabled
// this topic (mandatory for topicUserCreated/topicUserPasswordReset), and —
// for optional topics — whether the recipient has individually opted out.
func (p *smtpPlugin) handleEvent(topic string) plugin.EventHandler {
	return func(evt *plugin.Event) {
		cfg, passwordEnc, err := p.loadConfig()
		if err != nil {
			p.log.Error("handleEvent(" + topic + "): load config: " + err.Error())
			return
		}
		if !cfg.isConfigured() {
			return
		}

		mandatory := topic == topicUserCreated || topic == topicUserPasswordReset
		if !mandatory && !containsString(cfg.EnabledEvents, topic) {
			return
		}

		// Read branding live, once per dispatch — this (not any earlier
		// snapshot) is what makes an admin's logo/color change take effect
		// immediately for every email sent afterward.
		branding, _ := plugin.GetBranding()

		var recipientUserID, recipientEmail, recipientName, subject, htmlBody, textBody string

		switch topic {
		case topicUserCreated, topicUserPasswordReset:
			data, err := plugin.JSONPayload[passwordSetEventPayload](evt)
			if err != nil || data.Email == "" {
				return
			}
			// The event carries no token — mint one now, scoped to exactly
			// this user_id, only because this plugin has decided (SMTP
			// configured, mandatory topic) that it's actually going to
			// deliver a set-password link.
			token, err := p.issuePasswordSetToken(data.UserID) //nolint:staticcheck // SA4023: native (!wasip1) stub always errors; wasip1 build can return nil
			if err != nil { //nolint:staticcheck // SA4023: native (!wasip1) stub always errors; wasip1 build can return nil
				p.log.Error("handleEvent(" + topic + "): issue password set token: " + err.Error())
				return
			}
			publicURL, _ := p.cfg.Get("PUBLIC_URL")
			inviteURL := strings.TrimRight(publicURL, "/") + "/set-password?token=" + token

			recipientUserID, recipientEmail, recipientName = data.UserID, data.Email, data.FullName
			subject, htmlBody, textBody = renderPasswordSetEmail(branding, topic, data, inviteURL)
		case topicNotificationAssigned, topicNotificationMentioned, topicNotificationDocMentioned, topicNotificationTaskDescMentioned:
			data, err := plugin.JSONPayload[notificationPayload](evt)
			if err != nil || data.RecipientEmail == "" {
				return
			}
			recipientUserID, recipientEmail, recipientName = data.RecipientUserID, data.RecipientEmail, data.RecipientName
			subject, htmlBody, textBody = renderNotificationEmail(branding, topic, data)
		default:
			return
		}

		if !mandatory {
			disabled, err := p.loadUserPreferences(recipientUserID)
			if err == nil && containsString(disabled, topic) {
				return
			}
		}

		password, err := p.decrypt(passwordEnc)
		if err != nil {
			p.log.Error("handleEvent(" + topic + "): decrypt password: " + err.Error())
			return
		}

		if err := p.sendEmail(sendEmailInput{ //nolint:staticcheck // SA4023: native (!wasip1) stub always errors; wasip1 build can return nil
			Host: cfg.Host, Port: cfg.Port, Username: cfg.Username, Password: password,
			UseTLS: cfg.UseTLS, From: cfg.FromAddress, FromName: cfg.FromName,
			To: recipientEmail, ToName: recipientName,
			Subject: subject, HTMLBody: htmlBody, TextBody: textBody,
		}); err != nil { //nolint:staticcheck // SA4023: native (!wasip1) stub always errors; wasip1 build can return nil
			p.log.Error("handleEvent(" + topic + "): send: " + err.Error())
			return
		}
		p.log.Info(fmt.Sprintf("sent %s email to %s", topic, recipientEmail))
	}
}
