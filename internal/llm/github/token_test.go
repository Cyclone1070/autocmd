package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTokenSource_Get(t *testing.T) {
	mux := http.NewServeMux()
	exchangeCount := 0

	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "Bearer gho_token", r.Header.Get("Authorization"))
		exchangeCount++

		resp := map[string]interface{}{
			"token":      "tid_test",
			"expires_at": time.Now().Add(time.Hour).Unix(),
		}
		json.NewEncoder(w).Encode(resp)
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	source := NewTokenSource("gho_token", ts.URL+"/token")

	// 1. First get
	token, err := source.Get(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "tid_test", token)
	assert.Equal(t, 1, exchangeCount)

	// 2. Second get (should be cached)
	token, err = source.Get(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "tid_test", token)
	assert.Equal(t, 1, exchangeCount)
}
