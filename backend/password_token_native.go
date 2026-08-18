//go:build !wasip1

package main

import "errors"

// issuePasswordSetToken always fails outside a WASM module — matches
// sendEmail's native stub so `go build`/`go test` on the native host
// platform still compile and run.
func (p *smtpPlugin) issuePasswordSetToken(_ string) (string, error) {
	return "", errors.New("password_set_token_issue: only available when running as a WASM plugin")
}
