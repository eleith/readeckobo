package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		config  map[string]any
		wantErr bool
	}{
		{
			name: "valid config",
			config: map[string]any{
				"readeck": map[string]any{
					"host":        "https://readeck.example.com",
					"access_token": "test-access-token",
				},
				"server": map[string]any{
					"port": 8080,
				},
				"kobo": map[string]any{
					"serial": "test-serial",
				},
			},
			wantErr: false,
		},
		{
			name: "invalid config missing readeck.host",
			config: map[string]any{
				"readeck": map[string]any{
					"access_token": "test-access-token",
				},
				"kobo": map[string]any{
					"serial": "test-serial",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid config missing readeck.access_token",
			config: map[string]any{
				"readeck": map[string]any{
					"host": "https://readeck.example.com",
				},
				"kobo": map[string]any{
					"serial": "test-serial",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid config missing kobo.serial",
			config: map[string]any{
				"readeck": map[string]any{
					"host":        "https://readeck.example.com",
					"access_token": "test-access-token",
				},
				"server": map[string]any{
					"port": 8080,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid server.port too high",
			config: map[string]any{
				"readeck": map[string]any{
					"host":        "https://readeck.example.com",
					"access_token": "test-access-token",
				},
				"server": map[string]any{
					"port": 65536,
				},
				"kobo": map[string]any{
					"serial": "test-serial",
				},
			},
			wantErr: true,
		},
		{
			name: "valid server.port",
			config: map[string]any{
				"readeck": map[string]any{
					"host":        "https://readeck.example.com",
					"access_token": "test-access-token",
				},
				"server": map[string]any{
					"port": 8080,
				},
				"kobo": map[string]any{
					"serial": "test-serial",
				},
			},
			wantErr: false,
		},
		{
			name: "invalid readeck.host format",
			config: map[string]any{
				"readeck": map[string]any{
					"host":        "invalid-url",
					"access_token": "test-access-token",
				},
				"kobo": map[string]any{
					"serial": "test-serial",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "config-test")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer func() {
				if err := os.RemoveAll(tmpDir); err != nil {
					t.Errorf("Failed to remove temp dir: %v", err)
				}
			}()

			configPath := filepath.Join(tmpDir, "config.yaml")
			data, err := yaml.Marshal(tt.config)
			if err != nil {
				t.Fatalf("Failed to marshal test config: %v", err)
			}

			if err := os.WriteFile(configPath, data, 0644); err != nil {
				t.Fatalf("Failed to write dummy config file: %v", err)
			}

			_, err = Load(configPath)

			if (err != nil) != tt.wantErr {
				t.Errorf("Load() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}
