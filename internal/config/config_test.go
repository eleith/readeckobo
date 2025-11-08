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
					"host": "https://readeck.example.com",
				},
				"server": map[string]any{
					"port": 8080,
				},
				"users": []map[string]any{
					{
						"token":                "test-token",
						"readeck_access_token": "test-readeck-token",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid config missing readeck.host",
			config: map[string]any{
				"users": []map[string]any{
					{
						"token":                "test-token",
						"readeck_access_token": "test-readeck-token",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid config missing users",
			config: map[string]any{
				"readeck": map[string]any{
					"host": "https://readeck.example.com",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid server.port too high",
			config: map[string]any{
				"readeck": map[string]any{
					"host": "https://readeck.example.com",
				},
				"server": map[string]any{
					"port": 65536,
				},
				"users": []map[string]any{
					{
						"token":                "test-token",
						"readeck_access_token": "test-readeck-token",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "valid server.port",
			config: map[string]any{
				"readeck": map[string]any{
					"host": "https://readeck.example.com",
				},
				"server": map[string]any{
					"port": 8080,
				},
				"users": []map[string]any{
					{
						"token":                "test-token",
						"readeck_access_token": "test-readeck-token",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid readeck.host format",
			config: map[string]any{
				"readeck": map[string]any{
					"host": "invalid-url",
				},
				"users": []map[string]any{
					{
						"token":                "test-token",
						"readeck_access_token": "test-readeck-token",
					},
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
