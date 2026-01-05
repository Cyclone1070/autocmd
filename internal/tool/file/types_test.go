package file

import (
	"testing"

	"github.com/Cyclone1070/iav/internal/config"
)

func TestWriteFileRequest_Validation(t *testing.T) {
	cfg := config.DefaultConfig()

	tests := []struct {
		name    string
		req     WriteFileRequest
		wantErr bool
	}{
		{"Valid", WriteFileRequest{Path: "file.txt", Content: "content"}, false},
		{"EmptyPath", WriteFileRequest{Path: "", Content: "content"}, true},
		{"EmptyContent", WriteFileRequest{Path: "file.txt", Content: ""}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate(cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
