package main

import (
	"fmt"
	"html"
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

// emailLayoutData is the data buildLayoutHTML renders. Paragraphs are plain
// strings — buildLayoutHTML escapes them, so callers never build raw HTML
// fragments by hand.
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

// escapeHTML escapes s for safe interpolation into HTML text content or a
// double-quoted HTML attribute value.
func escapeHTML(s string) string {
	return html.EscapeString(s)
}

// safeURL returns s unchanged (HTML-escaped) if it's safe to embed in an
// href/src attribute — http(s), protocol-relative, or a path/fragment — and
// a harmless "#" placeholder otherwise. Rejects a javascript:/data:-style
// scheme, the same protection html/template's contextual auto-escaper
// applied to a URL-valued attribute before this file stopped depending on
// it (see the package comment on why: TinyGo's reflect implementation can't
// run html/template's own bootstrap).
func safeURL(s string) string {
	trimmed := strings.TrimSpace(s)
	lower := strings.ToLower(trimmed)
	switch {
	case trimmed == "":
		return ""
	case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"),
		strings.HasPrefix(trimmed, "//"), strings.HasPrefix(trimmed, "/"), strings.HasPrefix(trimmed, "#"):
		return html.EscapeString(trimmed)
	default:
		return "#"
	}
}

// safeHexColor returns s unchanged if it's a valid #rgb or #rrggbb hex
// color, and fallback otherwise. PrimaryColor is interpolated directly into
// a CSS declaration (background-color:...;), a context HTML-escaping alone
// doesn't protect: a value like "red;background:url(evil)" contains no HTML
// special characters but would inject arbitrary CSS. Restricting to a
// validated hex color sidesteps needing real CSS-value escaping.
func safeHexColor(s, fallback string) string {
	if len(s) != 4 && len(s) != 7 {
		return fallback
	}
	if s[0] != '#' {
		return fallback
	}
	for _, c := range s[1:] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return fallback
		}
	}
	return s
}

// buildLayoutHTML renders the shared email chrome: logo/brand header,
// heading + paragraphs, an optional brand-colored CTA button, and a footer.
// Table-based layout with inline styles throughout for email-client
// compatibility (most clients strip <style> blocks and many ignore
// border-radius/box-shadow — both are used here only as graceful-degrading
// touches, matching the web app's rounded-corner, low-shadow "High-Contrast
// Minimalism" light-mode look — not load-bearing for legibility).
// Colors/fonts are pulled directly from apps/web/src/index.css's :root
// (light) block; see the color* consts above and brandFrom's doc comment.
//
// Built directly with strings.Builder rather than html/template: every
// plugin in this org builds with TinyGo, and TinyGo's reflect package can't
// run text/template's builtin-function-map bootstrap (reflect.Type.NumOut()
// on a func value), which panics at runtime the moment any template — even
// one with no custom FuncMap — is first executed. Each field below is
// escaped for the specific context it's interpolated into (escapeHTML for
// text/attributes, safeURL for href/src, safeHexColor for the one CSS-value
// use), replicating what html/template's contextual auto-escaping did for
// free — see each helper's doc comment for why HTML-escaping alone isn't
// enough for the URL and color cases.
func buildLayoutHTML(data emailLayoutData) string {
	primaryColor := safeHexColor(data.PrimaryColor, defaultPrimaryColor)
	logoURL := safeURL(data.LogoURL)
	buttonURL := safeURL(data.ButtonURL)

	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n<meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\">\n<title>")
	b.WriteString(escapeHTML(data.Subject))
	// Same web fonts as the app (apps/web/src/index.css). Ignored by clients
	// that strip remote font loading in email — the inline font-family
	// stacks below already fall back to system sans-serif in that case.
	b.WriteString("</title>\n<link rel=\"stylesheet\" href=\"https://fonts.googleapis.com/css2?family=Syne:wght@700&family=DM+Sans:wght@400;500;600&display=swap\">\n</head>\n")
	b.WriteString("<body style=\"margin:0;padding:0;background-color:" + colorMuted + ";\">\n")
	b.WriteString("<table role=\"presentation\" width=\"100%\" cellpadding=\"0\" cellspacing=\"0\" style=\"background-color:" + colorMuted + ";padding:32px 16px;\">\n<tr><td align=\"center\">\n")
	b.WriteString("<table role=\"presentation\" width=\"100%\" cellpadding=\"0\" cellspacing=\"0\" style=\"max-width:480px;background-color:" + colorCard + ";border-radius:12px;border:1px solid " + colorBorder + ";\">\n")
	b.WriteString("<tr><td style=\"padding:28px 32px 24px 32px;border-bottom:1px solid " + colorBorder + ";\">\n")
	b.WriteString("<table role=\"presentation\" cellpadding=\"0\" cellspacing=\"0\" style=\"margin:0 auto;\"><tr>\n")
	b.WriteString("<td style=\"padding-right:10px;vertical-align:middle;\"><img src=\"")
	b.WriteString(logoURL)
	b.WriteString("\" alt=\"")
	b.WriteString(escapeHTML(data.BrandName))
	b.WriteString("\" width=\"36\" height=\"36\" style=\"display:block;width:36px;height:36px;border:0;\"></td>\n")
	b.WriteString("<td style=\"vertical-align:middle;\"><span style=\"font-family:'Syne',ui-sans-serif,sans-serif;font-size:19px;font-weight:700;color:" + colorForeground + ";\">")
	b.WriteString(escapeHTML(data.BrandName))
	b.WriteString("</span></td>\n</tr></table>\n</td></tr>\n")
	b.WriteString("<tr><td style=\"padding:32px 32px 8px 32px;font-family:'DM Sans',ui-sans-serif,system-ui,sans-serif;\">\n")
	b.WriteString("<h1 style=\"margin:0 0 14px 0;font-size:18px;line-height:1.4;color:" + colorForeground + ";font-weight:600;\">")
	b.WriteString(escapeHTML(data.Heading))
	b.WriteString("</h1>\n")
	for _, p := range data.Paragraphs {
		b.WriteString("<p style=\"margin:0 0 14px 0;font-size:14px;line-height:1.6;color:" + colorMutedForeground + ";\">")
		b.WriteString(escapeHTML(p))
		b.WriteString("</p>\n")
	}
	b.WriteString("</td></tr>\n")
	if data.ButtonURL != "" {
		b.WriteString("<tr><td style=\"padding:8px 32px 32px 32px;font-family:'DM Sans',ui-sans-serif,system-ui,sans-serif;\">\n")
		b.WriteString("<table role=\"presentation\" cellpadding=\"0\" cellspacing=\"0\"><tr><td style=\"border-radius:8px;background-color:")
		b.WriteString(primaryColor)
		b.WriteString(";\">\n<a href=\"")
		b.WriteString(buttonURL)
		b.WriteString("\" style=\"display:inline-block;padding:10px 22px;font-size:14px;font-weight:600;color:" + colorPrimaryFg + ";text-decoration:none;\">")
		b.WriteString(escapeHTML(data.ButtonLabel))
		b.WriteString("</a>\n</td></tr></table>\n")
		b.WriteString("<p style=\"margin:16px 0 0 0;font-size:12px;line-height:1.5;color:" + colorMutedForeground + ";word-break:break-all;\">")
		b.WriteString(escapeHTML(data.ButtonURL))
		b.WriteString("</p>\n</td></tr>")
	}
	b.WriteString("\n<tr><td style=\"padding:18px 32px;background-color:" + colorMuted + ";border-top:1px solid " + colorBorder + ";border-radius:0 0 12px 12px;font-family:'DM Sans',ui-sans-serif,system-ui,sans-serif;\">\n")
	b.WriteString("<p style=\"margin:0;font-size:12px;line-height:1.5;color:" + colorMutedForeground + ";\">")
	b.WriteString(escapeHTML(data.FooterText))
	b.WriteString("</p>\n</td></tr>\n</table>\n</td></tr>\n</table>\n</body>\n</html>\n")
	return b.String()
}

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
	return buildLayoutHTML(data)
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
