package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBookSyncConfiguration(t *testing.T) {
	tests := []struct {
		name            string
		extra           string
		userExtra       string
		wantErr         string
		wantBookSyncURL string
		wantPublicURL   string
		wantRemove      bool
	}{
		{name: "no optional settings"},
		{name: "old disabled switch is ignored", extra: "book_sync: false\n"},
		{name: "old enabled switch is ignored", extra: "book_sync: true\n"},
		{name: "custom sync endpoint with path", userExtra: "    book_sync_url: https://books.example.com/api/kobo/secret\n", wantBookSyncURL: "https://books.example.com/api/kobo/secret"},
		{name: "http sync endpoint is allowed", userExtra: "    book_sync_url: http://localhost:8081/kobo\n", wantBookSyncURL: "http://localhost:8081/kobo"},
		{name: "public URL and archived removal", extra: "public_url: https://reader.example.com/\n", userExtra: "    remove_archived_from_kobo: true\n", wantPublicURL: "https://reader.example.com/", wantRemove: true},
		{name: "invalid sync scheme", userExtra: "    book_sync_url: ftp://books.example.com\n", wantErr: "BookSyncURL"},
		{name: "relative sync URL", userExtra: "    book_sync_url: /api/kobo/device\n", wantErr: "BookSyncURL"},
		{name: "sync URL missing host", userExtra: "    book_sync_url: https:/kobo\n", wantErr: "BookSyncURL"},
		{name: "invalid sync port", userExtra: "    book_sync_url: https://books.example.com:bad/kobo\n", wantErr: "BookSyncURL"},
		{name: "invalid public scheme", extra: "public_url: ftp://reader.example.com\n", wantErr: "PublicURL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := "readeck:\n  host: https://readeck.example.com\n" + tt.extra + "users:\n  - token: device\n    readeck_access_token: readeck\n" + tt.userExtra
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Load() error = %v, want error mentioning %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := cfg.Users[0].BookSyncURL; got != tt.wantBookSyncURL {
				t.Errorf("BookSyncURL = %q, want %q", got, tt.wantBookSyncURL)
			}
			if cfg.PublicURL != tt.wantPublicURL {
				t.Errorf("PublicURL = %q, want %q", cfg.PublicURL, tt.wantPublicURL)
			}
			if cfg.Users[0].RemoveArchivedFromKobo != tt.wantRemove {
				t.Errorf("RemoveArchivedFromKobo = %v, want %v", cfg.Users[0].RemoveArchivedFromKobo, tt.wantRemove)
			}
		})
	}
}
