package main

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"

	plugin "github.com/Paca-AI/plugin-sdk-go"
)

// Fallback brand values used whenever the admin hasn't customized branding
// (see GET /branding / plugin.GetBranding) — kept in sync with the web
// app's light-mode design tokens (apps/web/src/index.css's :root block,
// e.g. --primary/--foreground/--muted/--border) and its default logo asset.
const (
	defaultBrandName    = "Paca"
	defaultPrimaryColor = "#5a9e1c" // --primary
	defaultLogoURL      = "https://raw.githubusercontent.com/Paca-AI/paca/refs/heads/master/docs/assets/paca-logo.svg"

	colorForeground      = "#111111" // --foreground / --card-foreground — headings
	colorMutedForeground = "#737373" // --muted-foreground — body copy, footer
	colorBorder          = "#d4d4d4" // --border
	colorMuted           = "#f5f5f5" // --muted — page background, footer background
	colorCard            = "#ffffff" // --card
	colorPrimaryFg       = "#ffffff" // --primary-foreground
)

// emailLayoutData is the data the shared layout template renders. Paragraphs
// are plain strings — html/template auto-escapes them, so callers never
// build raw HTML fragments by hand.
type emailLayoutData struct {
	Subject      string
	BrandName    string
	LogoURL      string
	PrimaryColor string
	Heading      string
	Paragraphs   []string
	ButtonURL    string
	ButtonLabel  string
	FooterText   string
}

// layoutHTML is the shared email chrome: logo/brand header, heading +
// paragraphs, an optional brand-colored CTA button, and a footer. Table-based
// layout with inline styles throughout for email-client compatibility (most
// clients strip <style> blocks and many ignore border-radius/box-shadow —
// both are used here only as graceful-degrading touches, matching the web
// app's rounded-corner, low-shadow "High-Contrast Minimalism" light-mode
// look — not load-bearing for legibility). Colors/fonts are pulled directly
// from apps/web/src/index.css's :root (light) block; see the color* consts
// above and brandFrom's doc comment.
var layoutHTML = template.Must(template.New("layout").Parse(`<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>{{.Subject}}</title>
<!-- Same web fonts as the app (apps/web/src/index.css). Ignored by clients
     that strip remote font loading in email — the inline font-family
     stacks below already fall back to system sans-serif in that case. -->
<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Syne:wght@700&family=DM+Sans:wght@400;500;600&display=swap">
</head>
<body style="margin:0;padding:0;background-color:` + colorMuted + `;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color:` + colorMuted + `;padding:32px 16px;">
<tr><td align="center">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:480px;background-color:` + colorCard + `;border-radius:12px;border:1px solid ` + colorBorder + `;">
<tr><td style="padding:28px 32px 24px 32px;border-bottom:1px solid ` + colorBorder + `;">
<table role="presentation" cellpadding="0" cellspacing="0" style="margin:0 auto;"><tr>
<td style="padding-right:10px;vertical-align:middle;"><img src="{{.LogoURL}}" alt="{{.BrandName}}" width="36" height="36" style="display:block;width:36px;height:36px;border:0;"></td>
<td style="vertical-align:middle;"><span style="font-family:'Syne',ui-sans-serif,sans-serif;font-size:19px;font-weight:700;color:` + colorForeground + `;">{{.BrandName}}</span></td>
</tr></table>
</td></tr>
<tr><td style="padding:32px 32px 8px 32px;font-family:'DM Sans',ui-sans-serif,system-ui,sans-serif;">
<h1 style="margin:0 0 14px 0;font-size:18px;line-height:1.4;color:` + colorForeground + `;font-weight:600;">{{.Heading}}</h1>
{{range .Paragraphs}}<p style="margin:0 0 14px 0;font-size:14px;line-height:1.6;color:` + colorMutedForeground + `;">{{.}}</p>
{{end}}</td></tr>
{{if .ButtonURL}}<tr><td style="padding:8px 32px 32px 32px;font-family:'DM Sans',ui-sans-serif,system-ui,sans-serif;">
<table role="presentation" cellpadding="0" cellspacing="0"><tr><td style="border-radius:8px;background-color:{{.PrimaryColor}};">
<a href="{{.ButtonURL}}" style="display:inline-block;padding:10px 22px;font-size:14px;font-weight:600;color:` + colorPrimaryFg + `;text-decoration:none;">{{.ButtonLabel}}</a>
</td></tr></table>
<p style="margin:16px 0 0 0;font-size:12px;line-height:1.5;color:` + colorMutedForeground + `;word-break:break-all;">{{.ButtonURL}}</p>
</td></tr>{{end}}
<tr><td style="padding:18px 32px;background-color:` + colorMuted + `;border-top:1px solid ` + colorBorder + `;border-radius:0 0 12px 12px;font-family:'DM Sans',ui-sans-serif,system-ui,sans-serif;">
<p style="margin:0;font-size:12px;line-height:1.5;color:` + colorMutedForeground + `;">{{.FooterText}}</p>
</td></tr>
</table>
</td></tr>
</table>
</body>
</html>
`))

// brandFrom resolves display brand values, falling back to the app's own
// defaults (its default primary color and logo asset) for anything the
// admin hasn't customized via workspace branding settings — the email
// always shows a logo image, never a text-only wordmark.
func brandFrom(branding *plugin.BrandingInfo) (name, logoURL, primaryColor string) {
	name, logoURL, primaryColor = defaultBrandName, defaultLogoURL, defaultPrimaryColor
	if branding == nil {
		return name, logoURL, primaryColor
	}
	if branding.BrandName != "" {
		name = branding.BrandName
	}
	if branding.PrimaryColorLight != "" {
		primaryColor = branding.PrimaryColorLight
	}
	if branding.LogoURL != "" {
		logoURL = branding.LogoURL
	}
	return name, logoURL, primaryColor
}

// renderLayout executes the shared layout template, filling in brand
// defaults for whatever the admin hasn't customized.
func renderLayout(branding *plugin.BrandingInfo, subject, heading string, paragraphs []string, buttonURL, buttonLabel string) string {
	brandName, logoURL, primaryColor := brandFrom(branding)
	data := emailLayoutData{
		Subject:      subject,
		BrandName:    brandName,
		LogoURL:      logoURL,
		PrimaryColor: primaryColor,
		Heading:      heading,
		Paragraphs:   paragraphs,
		ButtonURL:    buttonURL,
		ButtonLabel:  buttonLabel,
		FooterText:   fmt.Sprintf("This email was sent by %s. If you weren't expecting it, you can safely ignore it.", brandName),
	}
	var buf bytes.Buffer
	if err := layoutHTML.Execute(&buf, data); err != nil {
		// Fall back to a minimal plain body rather than sending nothing.
		return "<p>" + template.HTMLEscapeString(heading) + "</p>"
	}
	return buf.String()
}

// renderText builds the plain-text alternative part, mirroring the HTML
// email's content in the same order.
func renderText(heading string, paragraphs []string, buttonURL string) string {
	var b strings.Builder
	b.WriteString(heading)
	b.WriteString("\n\n")
	for _, p := range paragraphs {
		b.WriteString(p)
		b.WriteString("\n\n")
	}
	if buttonURL != "" {
		b.WriteString(buttonURL)
		b.WriteString("\n")
	}
	return b.String()
}

// passwordSetCopy holds the topic-specific subject/heading/intro copy for
// renderPasswordSetEmail, mirroring notificationCopy's per-topic pattern
// below.
type passwordSetCopy struct {
	subject string
	heading string
	intro   string
}

func copyForPasswordSetTopic(topic, brandName, name string) passwordSetCopy {
	if topic == topicUserPasswordReset {
		return passwordSetCopy{
			subject: fmt.Sprintf("Your %s password was reset", brandName),
			heading: fmt.Sprintf("Your password was reset, %s", name),
			intro:   fmt.Sprintf("An administrator reset your password on %s. Click the button below to set a new password.", brandName),
		}
	}
	// topicUserCreated
	return passwordSetCopy{
		subject: fmt.Sprintf("Welcome to %s — set your password", brandName),
		heading: fmt.Sprintf("Welcome to %s, %s!", brandName, name),
		intro:   fmt.Sprintf("An account was created for you on %s. Click the button below to set your password and get started.", brandName),
	}
}

// renderPasswordSetEmail renders the mandatory user.created "welcome, set
// your password" email and the mandatory user.password_reset "your
// password was reset" email — the only two emails in this plugin that
// carry a token, not a password, in their link. inviteURL is built by the
// caller from a token freshly minted via issuePasswordSetToken, since the
// event payload itself never carries one (see passwordSetEventPayload's
// doc comment).
func renderPasswordSetEmail(branding *plugin.BrandingInfo, topic string, data passwordSetEventPayload, inviteURL string) (subject, html, text string) {
	brandName, _, _ := brandFrom(branding)
	name := data.FullName
	if name == "" {
		name = data.Username
	}
	c := copyForPasswordSetTopic(topic, brandName, name)
	paragraphs := []string{
		c.intro,
		"This link is valid for a limited time and can only be used once.",
	}
	html = renderLayout(branding, c.subject, c.heading, paragraphs, inviteURL, "Set your password")
	text = renderText(c.heading, paragraphs, inviteURL)
	return c.subject, html, text
}

// notificationCopy holds the topic-specific heading/body/button copy for
// renderNotificationEmail. heading doubles as the email subject — both use
// the same neutral, third-person phrasing ("A task was assigned to you",
// not "{brand} assigned you a task") since the brand is a mail sender, not
// the one who took the action.
type notificationCopy struct {
	heading     string
	paragraph   string
	buttonLabel string // "View task" / "View document"
}

func copyForNotificationTopic(topic string, data notificationPayload) notificationCopy {
	actor := data.ActorName
	if actor == "" {
		actor = "Someone"
	}
	switch topic {
	case topicNotificationMentioned:
		return notificationCopy{
			heading:     "You were mentioned",
			paragraph:   fmt.Sprintf("%s mentioned you in a comment.", actor),
			buttonLabel: "View task",
		}
	case topicNotificationDocMentioned:
		return notificationCopy{
			heading:     "You were mentioned",
			paragraph:   fmt.Sprintf("%s mentioned you in a document.", actor),
			buttonLabel: "View document",
		}
	case topicNotificationTaskDescMentioned:
		return notificationCopy{
			heading:     "You were mentioned",
			paragraph:   fmt.Sprintf("%s mentioned you in a task's description.", actor),
			buttonLabel: "View task",
		}
	default: // topicNotificationAssigned
		return notificationCopy{
			heading:     "A task was assigned to you",
			paragraph:   fmt.Sprintf("%s assigned a task to you.", actor),
			buttonLabel: "View task",
		}
	}
}

// renderNotificationEmail renders the optional assigned/mentioned-in-a-
// comment/mentioned-in-a-document/mentioned-in-a-task-description emails —
// each a light wrapper around the app's existing in-app notification
// content (see services/api's notification.Svc).
func renderNotificationEmail(branding *plugin.BrandingInfo, topic string, data notificationPayload) (subject, html, text string) {
	c := copyForNotificationTopic(topic, data)
	subject = c.heading
	paragraphs := []string{c.paragraph}
	html = renderLayout(branding, subject, c.heading, paragraphs, data.LinkURL, c.buttonLabel)
	text = renderText(c.heading, paragraphs, data.LinkURL)
	return subject, html, text
}

// renderTestEmail renders the admin's "send test email" sample message —
// no token/link, just confirms the SMTP config and current branding render
// correctly end to end.
func renderTestEmail(branding *plugin.BrandingInfo) (subject, html, text string) {
	brandName, _, _ := brandFrom(branding)
	subject = fmt.Sprintf("%s — test email", brandName)
	heading := "Your SMTP configuration works"
	paragraphs := []string{
		fmt.Sprintf("This is a test email from %s, sent using the SMTP settings you just saved.", brandName),
		"If you can read this, outbound email delivery is working correctly.",
	}
	html = renderLayout(branding, subject, heading, paragraphs, "", "")
	text = renderText(heading, paragraphs, "")
	return subject, html, text
}
