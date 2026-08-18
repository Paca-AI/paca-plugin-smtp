//go:build wasip1

package main

import (
	"errors"

	plugin "github.com/Paca-AI/plugin-sdk-go"
)

// paca.password_set_token_issue(reqPtr i64, reqLen i64, resPtrPtr i64, resLenPtr i64)
//
// Mints a single-use password-set token for a given user_id. Private to
// this plugin (not part of plugin-sdk-go) for the same reason send_email
// is: minting a bearer credential for another user's account isn't a
// generic plugin concern. The host gates it on this plugin's manifest
// declaring the "users.password_set_token.issue" permission, and never
// includes the token in the user.created event payload itself — this call
// is how a plugin that decides, having received that event, that it wants
// to deliver an invite link fetches one on demand.
//
//go:wasmimport paca password_set_token_issue
//go:noescape
func hostIssuePasswordSetToken(reqPtr, reqLen, resPtrPtr, resLenPtr int64)

type issuePasswordSetTokenRequest struct {
	UserID string `json:"user_id"`
}

type issuePasswordSetTokenResult struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
	Error     string `json:"error"`
}

// issuePasswordSetToken asks the host to mint a password-set token for
// userID, scoped to exactly that account.
func (p *smtpPlugin) issuePasswordSetToken(userID string) (string, error) {
	var res issuePasswordSetTokenResult
	if err := plugin.CallHostFunction(hostIssuePasswordSetToken, issuePasswordSetTokenRequest{UserID: userID}, &res); err != nil {
		return "", err
	}
	if res.Error != "" {
		return "", errors.New(res.Error)
	}
	return res.Token, nil
}
