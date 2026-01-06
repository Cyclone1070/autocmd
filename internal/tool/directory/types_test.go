package directory

import (
	"testing"

	"github.com/Cyclone1070/iav/internal/config"
)

func TestFindFileRequest_Validation(t *testing.T) {
	cfg := config.DefaultConfig()

	tests := []struct {
		name         string
		req          FindFileRequest
		wantErr      bool
		verifyValues func(t *testing.T, req FindFileRequest)
	}{
		{"Valid", FindFileRequest{Pattern: "*.txt"}, false, nil},
		{"EmptyPattern", FindFileRequest{Pattern: ""}, true, nil},
		{"NegativeOffset_Clamps", FindFileRequest{Pattern: "*.txt", Offset: -1}, false, func(t *testing.T, req FindFileRequest) {
			if req.Offset != 0 {
				t.Errorf("expected offset 0, got %d", req.Offset)
			}
		}},
		{"NegativeLimit_Defaults", FindFileRequest{Pattern: "*.txt", Limit: -1}, false, func(t *testing.T, req FindFileRequest) {
			if req.Limit != cfg.Tools.DefaultFindFileLimit {
				t.Errorf("expected default limit %d, got %d", cfg.Tools.DefaultFindFileLimit, req.Limit)
			}
		}},
		{"LimitExceedsMax_Caps", FindFileRequest{Pattern: "*.txt", Limit: cfg.Tools.MaxFindFileLimit + 1}, false, func(t *testing.T, req FindFileRequest) {
			if req.Limit != cfg.Tools.MaxFindFileLimit {
				t.Errorf("expected max limit %d, got %d", cfg.Tools.MaxFindFileLimit, req.Limit)
			}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate(cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && tt.verifyValues != nil {
				tt.verifyValues(t, tt.req)
			}
		})
	}
}
