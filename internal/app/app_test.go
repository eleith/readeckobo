package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	GetBookmarksFunc       func(ctx context.Context, site string, page int, isArchived *bool) ([]readeck.Bookmark, int, error)
	GetBookmarkDetailsFunc func(ctx context.Context, id string) (*readeck.Bookmark, error)
	GetBookmarkArticleFunc func(ctx context.Context, id string) (string, error)
	UpdateBookmarkFunc     func(ctx context.Context, id string, updates map[string]any) error
	CreateBookmarkFunc     func(ctx context.Context, bookmarkURL string) error
}

func (m *MockReadeckClient) GetBookmarksSync(ctx context.Context, since *time.Time) ([]readeck.BookmarkSync, error) {
	return m.GetBookmarksSyncFunc(ctx, since)
}

func (m *MockReadeckClient) GetBookmarks(ctx context.Context, site string, page int, isArchived *bool) ([]readeck.Bookmark, int, error) {
	return m.GetBookmarksFunc(ctx, site, page, isArchived)
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
	testCases := []struct {
		name                 string
		reqBody              *GetRequest
		mockBookmarksSync    []readeck.BookmarkSync
		mockBookmarkDetails  map[string]*readeck.Bookmark
		mockBookmarksSyncErr error
		mockBookmarkDetailsErr error
		expectedStatus       int
		expectedListSize     int
		expectedTotal        int
	}{
		{
			name:    "successful get",
			reqBody: &GetRequest{Count: "1"},
			mockBookmarksSync: []readeck.BookmarkSync{
				{ID: "1", Type: "update"},
			},
			mockBookmarkDetails: map[string]*readeck.Bookmark{
				"1": {
					ID:    "1",
					Title: "Test Bookmark",
					URL:   "http://example.com/bookmark1",
				},
			},
			expectedStatus:   http.StatusOK,
			expectedListSize: 1,
			expectedTotal:    1,
		},
		{
			name:    "delete sync type",
			reqBody: &GetRequest{Count: "1"},
			mockBookmarksSync: []readeck.BookmarkSync{
				{ID: "1", Type: "delete"},
			},
			expectedStatus:   http.StatusOK,
			expectedListSize: 1,
			expectedTotal:    1,
		},
		{
			name:    "get bookmark details error",
			reqBody: &GetRequest{Count: "1"},
			mockBookmarksSync: []readeck.BookmarkSync{
				{ID: "1", Type: "update"},
			},
			mockBookmarkDetailsErr: fmt.Errorf("details error"),
			expectedStatus:         http.StatusOK,
			expectedListSize:       0,
			expectedTotal:          1,
		},
		{
			name:    "get bookmark details nil",
			reqBody: &GetRequest{Count: "1"},
			mockBookmarksSync: []readeck.BookmarkSync{
				{ID: "1", Type: "update"},
			},
			mockBookmarkDetails: map[string]*readeck.Bookmark{
				"1": nil,
			},
			expectedStatus:   http.StatusOK,
			expectedListSize: 0,
			expectedTotal:    1,
		},
		{
			name:             "get bookmarks sync error",
			reqBody:          &GetRequest{Count: "1"},
			mockBookmarksSyncErr: fmt.Errorf("sync error"),
			expectedStatus:   http.StatusInternalServerError,
		},
		{
			name:             "invalid request body",
			reqBody:          nil, // This will cause a failure in json.Marshal
			expectedStatus:   http.StatusBadRequest,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := &MockReadeckClient{
				GetBookmarksSyncFunc: func(ctx context.Context, since *time.Time) ([]readeck.BookmarkSync, error) {
					return tc.mockBookmarksSync, tc.mockBookmarksSyncErr
				},
				GetBookmarkDetailsFunc: func(ctx context.Context, id string) (*readeck.Bookmark, error) {
					if tc.mockBookmarkDetailsErr != nil {
						return nil, tc.mockBookmarkDetailsErr
					}
					return tc.mockBookmarkDetails[id], nil
				},
			}

			app := NewApp(
				WithConfig(&config.Config{}),
				WithReadeckClient(mockClient),
			)

			var body io.Reader
			if tc.reqBody != nil {
				jsonBody, _ := json.Marshal(tc.reqBody)
				body = bytes.NewReader(jsonBody)
			} else {
				body = strings.NewReader("invalid json")
			}


			req := httptest.NewRequest(http.MethodPost, "/api/kobo/get", body)
			rr := httptest.NewRecorder()

			app.HandleKoboGet(rr, req)

			if rr.Code != tc.expectedStatus {
				t.Errorf("expected status %d, got %d", tc.expectedStatus, rr.Code)
			}

			if tc.expectedStatus == http.StatusOK {
				var resp KoboGetResponse
				if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if len(resp.List) != tc.expectedListSize {
					t.Errorf("expected %d item in list, got %d", tc.expectedListSize, len(resp.List))
				}
				if resp.Total != tc.expectedTotal {
					t.Errorf("expected total to be %d, got %d", tc.expectedTotal, resp.Total)
				}
			}
		})
	}
}

func TestHandleKoboDownload(t *testing.T) {
	mockClient := &MockReadeckClient{
		GetBookmarksFunc: func(ctx context.Context, site string, page int, isArchived *bool) ([]readeck.Bookmark, int, error) {
			if site == "example.com" && page == 1 && isArchived != nil && !*isArchived {
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
	var createdBookmarkURL string

	mockClient := &MockReadeckClient{
		UpdateBookmarkFunc: func(ctx context.Context, id string, updates map[string]any) error {
			updatedBookmarkID = id
			updatedBookmarkData = updates
			return nil
		},
		CreateBookmarkFunc: func(ctx context.Context, bookmarkURL string) error {
			createdBookmarkURL = bookmarkURL
			return nil
		},
	}

	app := NewApp(
		WithConfig(&config.Config{}),
		WithReadeckClient(mockClient),
	)

	testCases := []struct {
		name                string
		actions             []any
		expectedStatus      bool
		expectedResults     []bool
		expectedUpdatedID   string
		expectedUpdatedData map[string]any
		expectedCreatedURL  string
	}{
		{
			name: "archive action",
			actions: []any{
				map[string]any{"action": "archive", "item_id": "1"},
			},
			expectedStatus:    true,
			expectedResults:   []bool{true},
			expectedUpdatedID: "1",
			expectedUpdatedData: map[string]any{"is_archived": true},
		},
		{
			name: "readd action",
			actions: []any{
				map[string]any{"action": "readd", "item_id": "2"},
			},
			expectedStatus:    true,
			expectedResults:   []bool{true},
			expectedUpdatedID: "2",
			expectedUpdatedData: map[string]any{"is_archived": false},
		},
		{
			name: "favorite action",
			actions: []any{
				map[string]any{"action": "favorite", "item_id": "3"},
			},
			expectedStatus:    true,
			expectedResults:   []bool{true},
			expectedUpdatedID: "3",
			expectedUpdatedData: map[string]any{"is_marked": true},
		},
		{
			name: "unfavorite action",
			actions: []any{
				map[string]any{"action": "unfavorite", "item_id": "4"},
			},
			expectedStatus:    true,
			expectedResults:   []bool{true},
			expectedUpdatedID: "4",
			expectedUpdatedData: map[string]any{"is_marked": false},
		},
		{
			name: "delete action",
			actions: []any{
				map[string]any{"action": "delete", "item_id": "5"},
			},
			expectedStatus:    true,
			expectedResults:   []bool{true},
			expectedUpdatedID: "5",
			expectedUpdatedData: map[string]any{"is_deleted": true},
		},
		{
			name: "add action",
			actions: []any{
				map[string]any{"action": "add", "url": "http://example.com/new"},
			},
			expectedStatus:     true,
			expectedResults:    []bool{true},
			expectedCreatedURL: "http://example.com/new",
		},
		{
			name: "unknown action",
			actions: []any{
				map[string]any{"action": "unknown", "item_id": "6"},
			},
			expectedStatus:  false,
			expectedResults: []bool{false},
		},
		{
			name: "invalid action",
			actions: []any{
				"invalid action",
			},
			expectedStatus:  false,
			expectedResults: []bool{false},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset mock data
			updatedBookmarkID = ""
			updatedBookmarkData = nil
			createdBookmarkURL = ""

			reqBody := SendRequest{Actions: tc.actions}
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

			if status, _ := resp["status"].(bool); status != tc.expectedStatus {
				t.Errorf("expected status %v, got %v", tc.expectedStatus, status)
			}

			results, ok := resp["action_results"].([]any)
			if !ok || len(results) != len(tc.expectedResults) {
				t.Fatalf("expected action_results to be a slice of length %d", len(tc.expectedResults))
			}
			for i, res := range results {
				if res.(bool) != tc.expectedResults[i] {
					t.Errorf("expected action_result[%d] to be %v, got %v", i, tc.expectedResults[i], res)
				}
			}

			if tc.expectedUpdatedID != "" && updatedBookmarkID != tc.expectedUpdatedID {
				t.Errorf("expected updated bookmark ID to be '%s', got '%s'", tc.expectedUpdatedID, updatedBookmarkID)
			}

			if tc.expectedUpdatedData != nil {
				for k, v := range tc.expectedUpdatedData {
					if updatedBookmarkData[k] != v {
						t.Errorf("expected updated data for key '%s' to be %v, got %v", k, v, updatedBookmarkData[k])
					}
				}
			}

			if tc.expectedCreatedURL != "" && createdBookmarkURL != tc.expectedCreatedURL {
				t.Errorf("expected created bookmark URL to be '%s', got '%s'", tc.expectedCreatedURL, createdBookmarkURL)
			}
		})
	}
}

func TestHandleKoboGetWithArchived(t *testing.T) {
	mockClient := &MockReadeckClient{
		GetBookmarksSyncFunc: func(ctx context.Context, since *time.Time) ([]readeck.BookmarkSync, error) {
			return []readeck.BookmarkSync{
				{ID: "1", Type: "update"},
				{ID: "2", Type: "update"},
			}, nil
		},
		GetBookmarkDetailsFunc: func(ctx context.Context, id string) (*readeck.Bookmark, error) {
			wordCount := 100
			switch id {
			case "1":
				return &readeck.Bookmark{
					ID:         "1",
					Title:      "Test Bookmark",
					URL:        "http://example.com/bookmark1",
					Href:       "http://example.com/bookmark1",
					WordCount:  &wordCount,
					IsArchived: false,
				}, nil
			case "2":
				return &readeck.Bookmark{
					ID:         "2",
					Title:      "Archived Bookmark",
					URL:        "http://example.com/bookmark2",
					Href:       "http://example.com/bookmark2",
					WordCount:  &wordCount,
					IsArchived: true,
				}, nil
			}
			return nil, fmt.Errorf("bookmark not found")
		},
	}

	app := NewApp(
		WithConfig(&config.Config{}),
		WithReadeckClient(mockClient),
	)

	reqBody := GetRequest{Count: "10"} // Request more than the number of bookmarks
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
	if resp.Total != 1 {
		t.Errorf("expected total to be 1, got %d", resp.Total)
	}
	if _, ok := resp.List["2"]; ok {
		t.Error("archived bookmark should not be in the list")
	}
}

func TestHandleConvertImage(t *testing.T) {
	app := NewApp(WithConfig(&config.Config{}))

	// Mock server to serve a test image
	imgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A simple 1x1 red PNG
		w.Header().Set("Content-Type", "image/png")
		if _, err := w.Write([]byte{
				0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
				0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
				0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xde, 0x00, 0x00, 0x00,
				0x0c, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
				0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
				0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
			}); err != nil {
				t.Fatalf("Failed to write response: %v", err)
			}
	}))
	defer imgSrv.Close()

	t.Run("successful conversion", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/convert-image?url="+imgSrv.URL, nil)
		rr := httptest.NewRecorder()

		app.HandleConvertImage(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
		}
		if rr.Header().Get("Content-Type") != "image/jpeg" {
			t.Errorf("expected content type image/jpeg, got %s", rr.Header().Get("Content-Type"))
		}
	})

	t.Run("missing url", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/convert-image", nil)
		rr := httptest.NewRecorder()

		app.HandleConvertImage(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
		}
	})

	t.Run("image fetch failed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/convert-image?url=http://invalid-url", nil)
		rr := httptest.NewRecorder()

		app.HandleConvertImage(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
		}
		if rr.Header().Get("Content-Type") != "image/jpeg" {
			t.Errorf("expected content type image/jpeg, got %s", rr.Header().Get("Content-Type"))
		}
	})

	t.Run("image decode failed", func(t *testing.T) {
		// Mock server to serve invalid image data
		invalidImgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			if _, err := w.Write([]byte("invalid image data")); err != nil {
				t.Fatalf("Failed to write response: %v", err)
			}
		}))
		defer invalidImgSrv.Close()

		req := httptest.NewRequest(http.MethodGet, "/api/convert-image?url="+invalidImgSrv.URL, nil)
		rr := httptest.NewRecorder()

		app.HandleConvertImage(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
		}
		if rr.Header().Get("Content-Type") != "image/jpeg" {
			t.Errorf("expected content type image/jpeg, got %s", rr.Header().Get("Content-Type"))
		}
	})
}