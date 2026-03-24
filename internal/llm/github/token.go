package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const defaultExchangeURL = "https://api.github.com/copilot_internal/v2/token"

// TokenSource manages the exchange of a long-lived GitHub OAuth token 
// for short-lived Copilot session tokens.
type TokenSource struct {
	oauthToken  string
	exchangeURL string
	client      *http.Client

	mu           sync.Mutex
	sessionToken string
	expiresAt    time.Time
}

// NewTokenSource creates a new token source.
func NewTokenSource(oauthToken string, exchangeURL string) *TokenSource {
	if exchangeURL == "" {
		exchangeURL = defaultExchangeURL
	}
	return &TokenSource{
		oauthToken:  oauthToken,
		exchangeURL: exchangeURL,
		client:      http.DefaultClient,
	}
}

// tokenResponse is the JSON from api.github.com/copilot_internal/v2/token.
type tokenExchangeResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
}

// Get returns a valid session token, exchanging if necessary.
func (s *TokenSource) Get(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Check Cache
	if s.sessionToken != "" && time.Now().Before(s.expiresAt.Add(-2*time.Minute)) {
		return s.sessionToken, nil
	}

	// 2. Exchange
	req, err := http.NewRequestWithContext(ctx, "GET", s.exchangeURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+s.oauthToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "GithubCopilot/1.155.0")
	req.Header.Set("Editor-Version", "vscode/1.85.1")
	req.Header.Set("Editor-Plugin-Version", "copilot/1.155.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("copilot token exchange failed: %s", resp.Status)
	}

	var ter tokenExchangeResponse
	if err := json.NewDecoder(resp.Body).Decode(&ter); err != nil {
		return "", err
	}

	s.sessionToken = ter.Token
	s.expiresAt = time.Unix(ter.ExpiresAt, 0)

	return s.sessionToken, nil
}
