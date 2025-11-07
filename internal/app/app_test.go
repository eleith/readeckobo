package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"readeckobo/internal/config"
	"readeckobo/internal/readeck"
)

// MockReadeckClient is a mock implementation of the Readeck client for testing.
type MockReadeckClient struct {
	GetBookmarksSyncFunc   func(ctx context.Context, since *time.Time) ([]readeck.BookmarkSync, error)
	GetBookmarksFunc       func(ctx context.Context, site string, page int) ([]readeck.Bookmark, int, error)
	GetBookmarkDetailsFunc func(ctx context.Context, id string) (*readeck.Bookmark, error)
	GetBookmarkArticleFunc func(ctx context.Context, id string) (string, error)
	UpdateBookmarkFunc     func(ctx context.Context, id string, updates map[string]any) error
	CreateBookmarkFunc     func(ctx context.Context, bookmarkURL string) error
}

func (m *MockReadeckClient) GetBookmarksSync(ctx context.Context, since *time.Time) ([]readeck.BookmarkSync, error) {
	return m.GetBookmarksSyncFunc(ctx, since)
}

func (m *MockReadeckClient) GetBookmarks(ctx context.Context, site string, page int) ([]readeck.Bookmark, int, error) {
	return m.GetBookmarksFunc(ctx, site, page)
}

func (m *MockReadeckClient) GetBookmarkDetails(ctx context.Context, id string) (*readeck.Bookmark, error) {
	return m.GetBookmarkDetailsFunc(ctx, id)
}

func (m *MockReadeckClient) GetBookmarkArticle(ctx context.Context, id string) (string, error) {
	return m.GetBookmarkArticleFunc(ctx, id)
}

func (m *MockReadeckClient) UpdateBookmark(ctx context.Context, id string, updates map[string]any) error {
	return m.UpdateBookmarkFunc(ctx, id, updates)
}

func (m *MockReadeckClient) CreateBookmark(ctx context.Context, bookmarkURL string) error {
	return m.CreateBookmarkFunc(ctx, bookmarkURL)
}

func TestHandleKoboGet(t *testing.T) {
	mockClient := &MockReadeckClient{
		GetBookmarksSyncFunc: func(ctx context.Context, since *time.Time) ([]readeck.BookmarkSync, error) {
			return []readeck.BookmarkSync{
				{ID: "1", Type: "update"},
			}, nil
		},
		GetBookmarkDetailsFunc: func(ctx context.Context, id string) (*readeck.Bookmark, error) {
			wordCount := 100
			return &readeck.Bookmark{
				ID:        "1",
				Title:     "Test Bookmark",
				URL:       "http://example.com/bookmark1",
				Href:      "http://example.com/bookmark1",
				WordCount: &wordCount,
			}, nil
		},
	}

	app := NewApp(
		WithConfig(&config.Config{}),
		WithReadeckClient(mockClient),
	)

	reqBody := GetRequest{Count: "1"}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/kobo/get", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	app.HandleKoboGet(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp KoboGetResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if len(resp.List) != 1 {
		t.Errorf("expected 1 item in list, got %d", len(resp.List))
	}
}

func TestHandleKoboDownload(t *testing.T) {
	mockClient := &MockReadeckClient{
		GetBookmarksFunc: func(ctx context.Context, site string, page int) ([]readeck.Bookmark, int, error) {
			if site == "example.com" && page == 1 {
				return []readeck.Bookmark{
					{ID: "1", Title: "Test Article", URL: "http://example.com/article1"},
				}, 1, nil
			}
			return nil, 0, nil
		},
		GetBookmarkArticleFunc: func(ctx context.Context, id string) (string, error) {
			if id == "1" {
				return `<html><body><h1>Test Article</h1><img src="http://example.com/image.png"></body></html>`, nil
			}
			return "", fmt.Errorf("article not found")
		},
	}

	app := NewApp(
		WithConfig(&config.Config{}),
		WithReadeckClient(mockClient),
	)

	form := url.Values{}
	form.Add("url", "http://example.com/article1")
	req := httptest.NewRequest(http.MethodPost, "/api/kobo/download", strings.NewReader(form.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	app.HandleKoboDownload(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if _, ok := resp["article"]; !ok {
		t.Error("expected 'article' key in response")
	}
	if _, ok := resp["images"]; !ok {
		t.Error("expected 'images' key in response")
	}

	article, _ := resp["article"].(string)
	if !strings.Contains(article, "<!--IMG_0-->") {
		t.Error("expected image to be replaced with comment")
	}
}

func TestHandleKoboSend(t *testing.T) {
	var updatedBookmarkID string
	var updatedBookmarkData map[string]any
	mockClient := &MockReadeckClient{
		UpdateBookmarkFunc: func(ctx context.Context, id string, updates map[string]any) error {
			updatedBookmarkID = id
			updatedBookmarkData = updates
			return nil
		},
		CreateBookmarkFunc: func(ctx context.Context, bookmarkURL string) error {
			return nil
		},
	}

	app := NewApp(
		WithConfig(&config.Config{}),
		WithReadeckClient(mockClient),
	)

	actions := []any{
		map[string]any{"action": "archive", "item_id": "1"},
	}
	reqBody := SendRequest{Actions: actions}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/kobo/send", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	app.HandleKoboSend(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if status, _ := resp["status"].(bool); !status {
		t.Error("expected status to be true")
	}
	if results, ok := resp["action_results"].([]any); !ok || len(results) != 1 || !results[0].(bool) {
		t.Error("expected action_results to be [true]")
	}

	if updatedBookmarkID != "1" {
		t.Errorf("expected updated bookmark ID to be '1', got '%s'", updatedBookmarkID)
	}
	if isArchived, _ := updatedBookmarkData["is_archived"].(bool); !isArchived {
		t.Error("expected is_archived to be true")
	}
}