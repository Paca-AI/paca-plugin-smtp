//go:build wasip1

package main

import (
	"errors"

	plugin "github.com/Paca-AI/plugin-sdk-go"
)

// paca.send_email(reqPtr i64, reqLen i64, resPtrPtr i64, resLenPtr i64)
//
// Real SMTP dispatch requires a raw TCP/TLS socket, which WASI does not
// expose to guests — the host dials it directly, unsandboxed, on this
// plugin's behalf. This import is private to this plugin (not part of
// plugin-sdk-go) since SMTP dispatch isn't a generic plugin concern; the
// host still gates it on this plugin's manifest declaring the "email.send"
// permission.
//
//go:wasmimport paca send_email
//go:noescape
func hostSendEmail(reqPtr, reqLen, resPtrPtr, resLenPtr int64)

type sendEmailResult struct {
	Error string `json:"error"`
}

// sendEmail dispatches an email through the host's SMTP client.
func (p *smtpPlugin) sendEmail(in sendEmailInput) error {
	var res sendEmailResult
	// Wrapped in a closure rather than passed directly: TinyGo requires a
	// go:wasmimport function to be called by name, not taken as a value.
	call := func(reqPtr, reqLen, resPtrPtr, resLenPtr int64) {
		hostSendEmail(reqPtr, reqLen, resPtrPtr, resLenPtr)
	}
	if err := plugin.CallHostFunction(call, in, &res); err != nil {
		return err
	}
	if res.Error != "" {
		return errors.New(res.Error)
	}
	return nil
}
