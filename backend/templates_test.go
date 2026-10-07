package main

import (
	"strings"
	"testing"

	plugin "github.com/Paca-AI/plugin-sdk-go"
)

// These specifically cover the escaping/validation buildLayoutHTML now does
// by hand (escapeHTML/safeURL/safeHexColor) in place of html/template's
// contextual auto-escaping — see templates.go's package comment for why.

func TestEscapeHTML(t *testing.T) {
	got := escapeHTML(`<script>alert("x")</script>&'`)
	for _, bad := range []string{"<script>", "</script>"} {
		if strings.Contains(got, bad) {
			t.Fatalf("escapeHTML left %q intact in %q", bad, got)
		}
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Fatalf("expected escaped output, got %q", got)
	}
}

func TestSafeURL(t *testing.T) {
	cases := []struct {
		in       string
		wantSafe bool
	}{
		{"https://paca.example.com/x", true},
		{"http://paca.example.com/x", true},
		{"/relative/path", true},
		{"#fragment", true},
		{"//protocol-relative.example.com", true},
		{"", true}, // empty is allowed through (renderLayout treats it as "no button")
		{"javascript:alert(1)", false},
		{"data:text/html,<script>alert(1)</script>", false},
		{"vbscript:msgbox(1)", false},
	}
	for _, c := range cases {
		got := safeURL(c.in)
		if c.wantSafe {
			if c.in != "" && got == "#" {
				t.Errorf("safeURL(%q) = %q, expected it to pass through", c.in, got)
			}
		} else {
			if got != "#" {
				t.Errorf("safeURL(%q) = %q, expected the unsafe placeholder \"#\"", c.in, got)
			}
		}
	}
}

func TestSafeHexColor(t *testing.T) {
	cases := []struct {
		in       string
		fallback string
		want     string
	}{
		{"#5a9e1c", "#000000", "#5a9e1c"},
		{"#fff", "#000000", "#fff"},
		{"red", "#000000", "#000000"},                          // named color rejected
		{"#5a9e1c;background:url(evil)", "#000000", "#000000"}, // CSS injection attempt rejected
		{"#12345", "#000000", "#000000"},                       // wrong length rejected
		{"", "#000000", "#000000"},                             // empty rejected
	}
	for _, c := range cases {
		if got := safeHexColor(c.in, c.fallback); got != c.want {
			t.Errorf("safeHexColor(%q, %q) = %q, want %q", c.in, c.fallback, got, c.want)
		}
	}
}

func TestBuildLayoutHTML_EscapesUserControlledFields(t *testing.T) {
	html := buildLayoutHTML(emailLayoutData{
		Subject:      `Hi <script>alert(1)</script>`,
		BrandName:    `Acme & Co`,
		LogoURL:      "javascript:alert(1)",
		PrimaryColor: "red;background:url(evil)",
		Heading:      "Hello",
		Paragraphs:   []string{`<img src=x onerror=alert(1)>`},
		ButtonURL:    "https://paca.example.com/go",
		ButtonLabel:  "Go",
		FooterText:   "bye",
	})

	if strings.Contains(html, "<script>") {
		t.Error("Subject's <script> tag was not escaped")
	}
	if strings.Contains(html, "<img src=x onerror=alert(1)>") {
		t.Error("paragraph content was not escaped")
	}
	if strings.Contains(html, `src="javascript:alert(1)"`) {
		t.Error("unsafe LogoURL scheme was not rejected")
	}
	if strings.Contains(html, "url(evil)") {
		t.Error("PrimaryColor CSS injection was not rejected")
	}
	if !strings.Contains(html, `href="https://paca.example.com/go"`) {
		t.Error("expected the safe ButtonURL to pass through into the href attribute")
	}
	if !strings.Contains(html, "Acme &amp; Co") {
		t.Error("expected BrandName's & to be entity-escaped")
	}
}

func TestBuildLayoutHTML_OmitsButtonBlockWhenNoButtonURL(t *testing.T) {
	html := buildLayoutHTML(emailLayoutData{Subject: "s", Heading: "h", FooterText: "f"})
	if strings.Contains(html, "<a href=") {
		t.Error("expected no button link when ButtonURL is empty")
	}
}

// Sanity check that brand-supplied values (not just literal test fixtures)
// flow through renderLayout's escaping too, exercising the same path
// renderTestEmail/renderNotificationEmail/renderPasswordSetEmail use.
func TestRenderLayout_EscapesBrandingFields(t *testing.T) {
	html := renderLayout(&plugin.BrandingInfo{
		BrandName:         `<b>Evil</b>`,
		PrimaryColorLight: "not-a-color",
		LogoURL:           "https://cdn.example.com/logo.png",
	}, "subject", "heading", []string{"para"}, "https://paca.example.com/go", "Go")

	if strings.Contains(html, "<b>Evil</b>") {
		t.Error("branding BrandName was not escaped")
	}
	if !strings.Contains(html, defaultPrimaryColor) {
		t.Error("expected invalid branding PrimaryColorLight to fall back to defaultPrimaryColor")
	}
}

// TestRenderNotificationEmail_IncludesEntityTitle covers the entity_title
// support: when the event carries the task/document title it must appear in
// both the subject and the body, and when it's empty the copy must fall back
// to the original title-less phrasing unchanged.
func TestRenderNotificationEmail_IncludesEntityTitle(t *testing.T) {
	const title = "Ship the landing page"

	cases := []struct {
		name     string
		topic    string
		wantText string // a phrase that must appear in the body paragraph when titled
	}{
		{"assigned", topicNotificationAssigned, "assigned you the task \"" + title + "\""},
		{"comment", topicNotificationMentioned, "mentioned you in a comment on \"" + title + "\""},
		{"doc", topicNotificationDocMentioned, "mentioned you in the document \"" + title + "\""},
		{"taskdesc", topicNotificationTaskDescMentioned, "in the description of \"" + title + "\""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := notificationPayload{ActorName: "Ada", EntityTitle: title, LinkURL: "https://paca.example/x"}
			subject, html, text := renderNotificationEmail(nil, tc.topic, data)

			if !strings.Contains(subject, title) {
				t.Errorf("subject should contain the title %q, got %q", title, subject)
			}
			if !strings.Contains(text, tc.wantText) {
				t.Errorf("plain-text body should contain %q, got %q", tc.wantText, text)
			}
			if !strings.Contains(html, title) {
				t.Errorf("html body should contain the title %q", title)
			}
		})
	}
}

func TestRenderNotificationEmail_FallsBackWithoutTitle(t *testing.T) {
	// No EntityTitle → original copy, no stray quotes, no title fragment.
	data := notificationPayload{ActorName: "Ada"}
	subject, _, text := renderNotificationEmail(nil, topicNotificationAssigned, data)

	if subject != "A task was assigned to you" {
		t.Errorf("expected the original title-less subject, got %q", subject)
	}
	if !strings.Contains(text, "Ada assigned a task to you.") {
		t.Errorf("expected the original title-less body, got %q", text)
	}
	if strings.Contains(text, "\"\"") {
		t.Errorf("empty title must not leave empty quotes in the body: %q", text)
	}
}
