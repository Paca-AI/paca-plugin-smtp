package main

// sendEmailInput is the JSON payload sent to the host's paca.send_email
// function (see mail_wasm.go). Field names are the wire contract with the
// host — see services/api's internal/platform/plugin/email.go for the
// matching decode side.
type sendEmailInput struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	// UseTLS selects implicit TLS on connect (typically port 465). When
	// false, the host still opportunistically upgrades via STARTTLS
	// (typically port 587/25) if the server offers it.
	UseTLS   bool   `json:"use_tls"`
	From     string `json:"from"`
	FromName string `json:"from_name"`
	To       string `json:"to"`
	ToName   string `json:"to_name"`
	Subject  string `json:"subject"`
	HTMLBody string `json:"html_body"`
	TextBody string `json:"text_body"`
}
