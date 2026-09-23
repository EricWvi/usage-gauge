package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

type claude struct {
	credentials string
	url         string
	client      *http.Client
}

type queryError struct {
	status     int
	message    string
	retryAfter string
}

func (e *queryError) Error() string { return e.message }

func newClaude(credentials string) *claude {
	return &claude{
		credentials: credentials,
		url:         "https://api.anthropic.com/api/oauth/usage",
		client: &http.Client{
			// Never forward local credentials to a redirected endpoint.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

func (c *claude) read(ctx context.Context) (json.RawMessage, error) {
	// Reopen on every query, including after Claude atomically replaces the file.
	// Token refresh belongs to Claude Code; this service never writes credentials.
	data, err := os.ReadFile(c.credentials)
	if err != nil {
		return nil, &queryError{status: 502, message: "Cannot read Claude credentials file"}
	}
	var credentials struct {
		OAuth struct {
			AccessToken string `json:"accessToken"`
		} `json:"claudeAiOauth"`
	}
	if json.Unmarshal(data, &credentials) != nil {
		return nil, &queryError{status: 502, message: "Invalid Claude credentials JSON"}
	}
	token := strings.TrimSpace(credentials.OAuth.AccessToken)
	if token == "" || strings.ContainsAny(token, "\r\n") {
		return nil, &queryError{status: 401, message: "Missing or invalid Claude OAuth access token; sign in with Claude Code"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return nil, &queryError{status: 502, message: "Cannot create Claude usage request"}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "claude-code/2.1.280")
	resp, err := c.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// Transport errors or upstream bodies may contain credentials: do not expose them.
		return nil, &queryError{status: 502, message: "Claude usage request failed"}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		status := http.StatusBadGateway
		message := fmt.Sprintf("Claude usage API returned HTTP %d", resp.StatusCode)
		if resp.StatusCode == 401 || resp.StatusCode == 403 || resp.StatusCode == 429 {
			status = resp.StatusCode
		}
		if status == 401 || status == 403 {
			message += "; refresh your login with Claude Code"
		}
		failure := &queryError{status: status, message: message}
		if status == 429 {
			failure.retryAfter = resp.Header.Get("Retry-After")
		}
		return nil, failure
	}
	const maxResponse = 1 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &queryError{status: 502, message: "Cannot read Claude usage response"}
	}
	var object map[string]json.RawMessage
	if len(body) > maxResponse || json.Unmarshal(body, &object) != nil || object == nil {
		return nil, &queryError{status: 502, message: "Invalid Claude usage response"}
	}
	return json.RawMessage(body), nil
}
