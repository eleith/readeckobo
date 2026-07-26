package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name        string
		config      map[string]any
		yamlContent string
		noFile      bool
		wantErr     bool
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
		{
			name:    "file does not exist",
			noFile:  true,
			wantErr: true,
		},
		{
			name:        "invalid yaml syntax",
			yamlContent: "readeck: host: [unbalanced brackets",
			wantErr:     true,
		},
		{
			name:        "invalid yaml structure for unmarshal",
			yamlContent: "readeck: \"should be a map but is a string\"",
			wantErr:     true,
		},
		{
			name: "valid config with book_sync",
			config: map[string]any{
				"readeck": map[string]any{
					"host": "https://readeck.example.com",
				},
				"book_sync": true,
				"users": []map[string]any{
					{
						"token":                "test-token",
						"readeck_access_token": "test-readeck-token",
						"book_service_url":     "https://grimmory.example.com/api/kobo/grimmory-token",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid config book_sync enabled without any book_service_url",
			config: map[string]any{
				"readeck": map[string]any{
					"host": "https://readeck.example.com",
				},
				"book_sync": true,
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

			if !tt.noFile {
				var data []byte
				if tt.yamlContent != "" {
					data = []byte(tt.yamlContent)
				} else {
					data, err = yaml.Marshal(tt.config)
					if err != nil {
						t.Fatalf("Failed to marshal test config: %v", err)
					}
				}

				if err := os.WriteFile(configPath, data, 0644); err != nil {
					t.Fatalf("Failed to write dummy config file: %v", err)
				}
			}

			_, err = Load(configPath)

			if (err != nil) != tt.wantErr {
				t.Errorf("Load() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func TestResolveBookSyncUpstream(t *testing.T) {
	tests := []struct {
		name         string
		cfg          Config
		deviceToken  string
		wantUpstream string
		wantOK       bool
	}{
		{
			name: "disabled returns false even with a configured url",
			cfg: Config{
				BookSync: false,
				Users: []User{
					{Token: "t1", BookServiceURL: "https://grimmory.example.com/api/kobo/g1"},
				},
			},
			deviceToken: "t1",
			wantOK:      false,
		},
		{
			name: "matching device token returns that user's endpoint",
			cfg: Config{
				BookSync: true,
				Users: []User{
					{Token: "t1", BookServiceURL: "https://grimmory.example.com/api/kobo/g1"},
					{Token: "t2", BookServiceURL: "https://grimmory.example.com/api/kobo/g2"},
				},
			},
			deviceToken:  "t2",
			wantUpstream: "https://grimmory.example.com/api/kobo/g2",
			wantOK:       true,
		},
		{
			name: "unknown device token returns false",
			cfg: Config{
				BookSync: true,
				Users: []User{
					{Token: "t1", BookServiceURL: "https://grimmory.example.com/api/kobo/g1"},
				},
			},
			deviceToken: "unknown-token",
			wantOK:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUpstream, gotOK := tt.cfg.ResolveBookSyncUpstream(tt.deviceToken)
			if gotOK != tt.wantOK {
				t.Errorf("ResolveBookSyncUpstream() ok = %v, want %v", gotOK, tt.wantOK)
			}
			if gotUpstream != tt.wantUpstream {
				t.Errorf("ResolveBookSyncUpstream() upstream = %q, want %q", gotUpstream, tt.wantUpstream)
			}
		})
	}
}
