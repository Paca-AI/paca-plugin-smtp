//go:build !wasip1

package main

import "errors"

// sendEmail always fails outside a WASM module — matches plugin-sdk-go's own
// convention for host-only capabilities (e.g. Fetch) so `go build`/`go test`
// on the native host platform still compile and run.
func (p *smtpPlugin) sendEmail(_ sendEmailInput) error {
	return errors.New("send_email: only available when running as a WASM plugin")
}
