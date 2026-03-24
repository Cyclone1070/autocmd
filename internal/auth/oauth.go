package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
)

// OAuthManager handles the RFC 8628 Device Authorization Flow.
type OAuthManager struct {
	client *http.Client
}

// NewOAuthManager creates a new OAuth manager.
func NewOAuthManager(client *http.Client) *OAuthManager {
	if client == nil {
		client = http.DefaultClient
	}
	return &OAuthManager{client: client}
}

// deviceCodeResponse is the JSON response from the device code endpoint.
type deviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// tokenResponse is the JSON response from the token endpoint.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	Error       string `json:"error"`
}

// RunDeviceFlow executes the polling flow based on a logicless OAuthMethod descriptor.
func (m *OAuthManager) RunDeviceFlow(ctx context.Context, cfg domain.OAuthMethod, onCode func(uri string, code string)) (string, error) {
	// 1. Request Device Code
	data := url.Values{}
	data.Set("client_id", cfg.ClientID)
	if len(cfg.Scopes) > 0 {
		// GitHub expects space-separated scopes
		// See: https://docs.github.com/en/apps/oauth-apps/maintaining-oauth-apps/scopes-for-oauth-apps
	}

	req, err := http.NewRequestWithContext(ctx, "POST", cfg.DeviceAuthURL, bytes.NewBufferString(data.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := m.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("device code request failed: %s", resp.Status)
	}

	var dcr deviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&dcr); err != nil {
		return "", err
	}

	// 2. Alert User
	onCode(dcr.VerificationURI, dcr.UserCode)

	// 3. Poll for Token
	interval := time.Duration(dcr.Interval) * time.Second
	if interval == 0 {
		interval = 5 * time.Second
	}

	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-timer.C:
			tokenData := url.Values{}
			tokenData.Set("client_id", cfg.ClientID)
			tokenData.Set("device_code", dcr.DeviceCode)
			tokenData.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")

			tr, err := m.postToken(ctx, cfg.TokenURL, tokenData)
			if err != nil {
				return "", err
			}

			if tr.AccessToken != "" {
				return tr.AccessToken, nil
			}

			switch tr.Error {
			case "authorization_pending":
				// Continue polling
			case "slow_down":
				interval += 5 * time.Second
			case "expired_token":
				return "", fmt.Errorf("session expired, please try again")
			case "access_denied":
				return "", fmt.Errorf("access denied by user")
			case "":
				// No error, but no token? 
			default:
				return "", fmt.Errorf("oauth error: %s", tr.Error)
			}

			timer.Reset(interval)
		}
	}
}

func (m *OAuthManager) postToken(ctx context.Context, uri string, data url.Values) (*tokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", uri, bytes.NewBufferString(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, err
	}
	return &tr, nil
}
