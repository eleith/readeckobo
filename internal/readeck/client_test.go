package readeck

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	client, err := NewClient("http://localhost:8080", "test-token")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	if client.BaseURL.String() != "http://localhost:8080" {
		t.Errorf("Expected BaseURL to be http://localhost:8080, got %s", client.BaseURL.String())
	}
	if client.AccessToken != "test-token" {
		t.Errorf("Expected AccessToken to be test-token, got %s", client.AccessToken)
	}

	// This should now correctly return an error due to stricter URL parsing
	_, err = NewClient("invalid-url", "test-token")
	if err == nil {
		t.Error("Expected error for invalid URL, got nil")
	}
}

func TestGetBookmarksSync(t *testing.T) {
	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/bookmarks/sync" {
			t.Errorf("Expected to request '/api/bookmarks/sync', got '%s'", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("Expected Authorization header 'Bearer test-token', got '%s'", r.Header.Get("Authorization"))
		}

				mockResponse := []BookmarkSync{
			{ID: "1", Time: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), Type: "update"},
		}
		if err := json.NewEncoder(w).Encode(mockResponse); err != nil {
			t.Fatalf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client, _ := NewClient(server.URL, "test-token")
	ctx := context.Background()

	syncEvents, err := client.GetBookmarksSync(ctx, nil)
	if err != nil {
		t.Fatalf("GetBookmarksSync failed: %v", err)
	}
	if len(syncEvents) != 1 || syncEvents[0].ID != "1" {
		t.Errorf("Expected 1 sync event with ID '1', got %+v", syncEvents)
	}
}

func TestGetBookmarks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/bookmarks" {
			t.Errorf("Expected to request '/api/bookmarks', got '%s'", r.URL.Path)
		}
		if r.URL.Query().Get("site") != "example.com" {
			t.Errorf("Expected site query parameter 'example.com', got '%s'", r.URL.Query().Get("site"))
		}
		if r.URL.Query().Get("page") != "1" {
			t.Errorf("Expected page query parameter '1', got '%s'", r.URL.Query().Get("page"))
		}

		mockResponse := []Bookmark{
			{ID: "b1", Title: "Test Bookmark"},
		}
		w.Header().Set("Total-Pages", "1")
		if err := json.NewEncoder(w).Encode(mockResponse); err != nil {
			t.Fatalf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client, _ := NewClient(server.URL, "test-token")
	ctx := context.Background()

	bookmarks, totalPages, err := client.GetBookmarks(ctx, "example.com", 1, nil)
	if err != nil {
		t.Fatalf("GetBookmarks failed: %v", err)
	}
	if len(bookmarks) != 1 || bookmarks[0].ID != "b1" {
		t.Errorf("Expected 1 bookmark with ID 'b1', got %+v", bookmarks)
	}
	if totalPages != 1 {
		t.Errorf("Expected totalPages to be 1, got %d", totalPages)
	}
}

func TestGetBookmarkDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/bookmarks/b1" {
			t.Errorf("Expected to request '/api/bookmarks/b1', got '%s'", r.URL.Path)
		}

		mockResponse := Bookmark{ID: "b1", Title: "Detailed Bookmark"}
		if err := json.NewEncoder(w).Encode(mockResponse); err != nil {
			t.Fatalf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client, _ := NewClient(server.URL, "test-token")
	ctx := context.Background()

	bookmark, err := client.GetBookmarkDetails(ctx, "b1")
	if err != nil {
		t.Fatalf("GetBookmarkDetails failed: %v", err)
	}
	if bookmark == nil || bookmark.ID != "b1" {
		t.Errorf("Expected bookmark with ID 'b1', got %+v", bookmark)
	}
}

func TestGetBookmarkArticle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/bookmarks/b1/article" {
			t.Errorf("Expected to request '/api/bookmarks/b1/article', got '%s'", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/html")
		if _, err := w.Write([]byte("<html><body><h1>Article Content</h1></body></html>")); err != nil {
			t.Fatalf("Failed to write response: %v", err)
		}
	}))
	defer server.Close()

	client, _ := NewClient(server.URL, "test-token")
	ctx := context.Background()

	article, err := client.GetBookmarkArticle(ctx, "b1")
	if err != nil {
		t.Fatalf("GetBookmarkArticle failed: %v", err)
	}
	expectedArticle := "<html><body><h1>Article Content</h1></body></html>"
	if article != expectedArticle {
		t.Errorf("Expected article '%s', got '%s'", expectedArticle, article)
	}
}

func TestUpdateBookmark(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("Expected PATCH method, got %s", r.Method)
		}
		if r.URL.Path != "/api/bookmarks/b1" {
			t.Errorf("Expected to request '/api/bookmarks/b1', got '%s'", r.URL.Path)
		}

		var updates map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}
		if updates["is_archived"] != true {
			t.Errorf("Expected is_archived to be true, got %v", updates["is_archived"])
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client, _ := NewClient(server.URL, "test-token")
	ctx := context.Background()

	updates := map[string]interface{}{"is_archived": true}
	err := client.UpdateBookmark(ctx, "b1", updates)
	if err != nil {
		t.Fatalf("UpdateBookmark failed: %v", err)
	}
}

func TestUpdateBookmarkNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, _ := NewClient(server.URL, "test-token")
	ctx := context.Background()

	updates := map[string]interface{}{"is_archived": true}
	err := client.UpdateBookmark(ctx, "nonexistent-id", updates)
	if err != nil {
		t.Errorf("Expected no error for 404 status, got %v", err)
	}
}

func TestCreateBookmark(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST method, got %s", r.Method)
		}
		if r.URL.Path != "/api/bookmarks" {
			t.Errorf("Expected to request '/api/bookmarks', got '%s'", r.URL.Path)
		}

		var body map[string]string
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}
		if body["url"] != "http://example.com/new" {
			t.Errorf("Expected URL 'http://example.com/new', got '%s'", body["url"])
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client, _ := NewClient(server.URL, "test-token")
	ctx := context.Background()

	err := client.CreateBookmark(ctx, "http://example.com/new")
	if err != nil {
		t.Fatalf("CreateBookmark failed: %v", err)
	}
}

func TestGetBookmarksWithIsArchived(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/bookmarks" {
			t.Errorf("Expected to request '/api/bookmarks', got '%s'", r.URL.Path)
		}
		if r.URL.Query().Get("site") != "example.com" {
			t.Errorf("Expected site query parameter 'example.com', got '%s'", r.URL.Query().Get("site"))
		}
		if r.URL.Query().Get("page") != "1" {
			t.Errorf("Expected page query parameter '1', got '%s'", r.URL.Query().Get("page"))
		}
		if r.URL.Query().Get("is_archived") != "false" {
			t.Errorf("Expected is_archived query parameter 'false', got '%s'", r.URL.Query().Get("is_archived"))
		}

		mockResponse := []Bookmark{
			{ID: "b1", Title: "Test Bookmark"},
		}
		w.Header().Set("Total-Pages", "1")
		if err := json.NewEncoder(w).Encode(mockResponse); err != nil {
			t.Fatalf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client, _ := NewClient(server.URL, "test-token")
	ctx := context.Background()

	isArchived := false
	bookmarks, totalPages, err := client.GetBookmarks(ctx, "example.com", 1, &isArchived)
	if err != nil {
		t.Fatalf("GetBookmarks failed: %v", err)
	}
	if len(bookmarks) != 1 || bookmarks[0].ID != "b1" {
		t.Errorf("Expected 1 bookmark with ID 'b1', got %+v", bookmarks)
	}
	if totalPages != 1 {
		t.Errorf("Expected totalPages to be 1, got %d", totalPages)
	}
}