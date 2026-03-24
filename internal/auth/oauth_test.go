package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Cyclone1070/iav/internal/auth"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestOAuthManager_RunDeviceFlow(t *testing.T) {
	mux := http.NewServeMux()

	// 1. Device Code Mock
	mux.HandleFunc("/device/code", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		resp := map[string]interface{}{
			"device_code":      "dev_123",
			"user_code":        "ABCD-1234",
			"verification_uri": "https://github.com/login/device",
			"expires_in":       60,
			"interval":         1,
		}
		json.NewEncoder(w).Encode(resp)
	})

	// 2. Token Mock (Success on second poll)
	polls := 0
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		polls++
		if polls < 2 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"access_token": "gho_test_token"})
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	mgr := auth.NewOAuthManager(http.DefaultClient)
	cfg := domain.OAuthMethod{
		ClientID:      "test_client",
		DeviceAuthURL: ts.URL + "/device/code",
		TokenURL:      ts.URL + "/token",
	}

	var capturedURI, capturedCode string
	token, err := mgr.RunDeviceFlow(context.Background(), cfg, func(uri, code string) {
		capturedURI = uri
		capturedCode = code
	})

	assert.NoError(t, err)
	assert.Equal(t, "gho_test_token", token)
	assert.Equal(t, "https://github.com/login/device", capturedURI)
	assert.Equal(t, "ABCD-1234", capturedCode)
	assert.Equal(t, 2, polls)
}
