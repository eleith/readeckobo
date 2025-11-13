package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url" // Added this import
	"strings"
	"testing"
	"time"

	"mime/multipart"
	"net/textproto"

	"readeckobo/internal/config"
	"readeckobo/internal/logger"
	"readeckobo/internal/models"
	"readeckobo/internal/readeck"
)

// MockRoundTripper is a mock implementation of http.RoundTripper for testing.
type MockRoundTripper struct {
	RoundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if m.RoundTripFunc != nil {
		return m.RoundTripFunc(req)
	}
	return nil, fmt.Errorf("mock RoundTripFunc not set")
}

var testLogger = logger.New(logger.DEBUG)

// Define a mock Kobo serial and a corresponding plaintext Readeck token
var mockDeviceToken = "mock-device-token"
var mockPlaintextReadeckToken = "mock_readeck_token_for_tests"

func TestCompareURLs(t *testing.T) {
	testCases := []struct {
		name     string
		url1     string
		url2     string
		expected bool
		hasError bool
	}{
		{
			name:     "exact match",
			url1:     "https://example.com/path/to/resource",
			url2:     "https://example.com/path/to/resource",
			expected: true,
			hasError: false,
		},
		{
			name:     "match with www. prefix on url1",
			url1:     "https://www.example.com/path",
			url2:     "https://example.com/path",
			expected: true,
			hasError: false,
		},
		{
			name:     "match with www. prefix on url2",
			url1:     "https://example.com/path",
			url2:     "https://www.example.com/path",
			expected: true,
			hasError: false,
		},
		{
			name:     "match with different query parameters",
			url1:     "https://example.com/path?param1=value1",
			url2:     "https://example.com/path?param2=value2",
			expected: true,
			hasError: false,
		},
		{
			name:     "match with different fragments",
			url1:     "https://example.com/path#section1",
			url2:     "https://example.com/path#section2",
			expected: true,
			hasError: false,
		},
		{
			name:     "match with different query params and fragments",
			url1:     "https://www.example.com/path?p1=v1#s1",
			url2:     "https://example.com/path?p2=v2#s2",
			expected: true,
			hasError: false,
		},
		{
			name:     "mismatch in path",
			url1:     "https://example.com/path1",
			url2:     "https://example.com/path2",
			expected: false,
			hasError: false,
		},
		{
			name:     "mismatch in host",
			url1:     "https://example.com/path",
			url2:     "https://anotherexample.com/path",
			expected: false,
			hasError: false,
		},
		{
			name:     "mismatch in scheme",
			url1:     "http://example.com/path",
			url2:     "https://example.com/path",
			expected: false,
			hasError: false,
		},
		{
			name:     "invalid url1 (relative path)",
			url1:     "invalid-url",
			url2:     "https://example.com/path",
			expected: false,
			hasError: false,
		},
		{
			name:     "invalid url2 (relative path)",
			url1:     "https://example.com/path",
			url2:     "invalid-url",
			expected: false,
			hasError: false,
		},
		{
			name:     "empty urls",
			url1:     "",
			url2:     "",
			expected: true, // Empty URLs are considered equal after parsing to base components
			hasError: false,
		},
		{
			name:     "url with trailing slash",
			url1:     "https://example.com/path/",
			url2:     "https://example.com/path",
			expected: false, // Paths must match exactly
			hasError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			match, err := compareURLs(tc.url1, tc.url2)

			if tc.hasError {
				if err == nil {
					t.Errorf("Expected an error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Did not expect an error but got: %v", err)
				}
				if match != tc.expected {
					t.Errorf("Expected match to be %v, but got %v", tc.expected, match)
				}
			}
		})
	}
}

func TestHandleKoboGet(t *testing.T) {
	// sinceValue is a valid timestamp for incremental sync tests.
	sinceValue := float64(1672531200) // 2023-01-01 00:00:00 UTC

	type koboGetTestCase struct {
		name                   string
		reqBody                *models.KoboGetRequest
		mockBookmarksSync      []readeck.BookmarkSync
		mockBookmarkDetails    map[string]*readeck.Bookmark
		mockBookmarksSyncErr   error
		mockBookmarkDetailsErr error
		expectedStatus         int
		expectedListSize       int
		expectedTotal          int
	}

	testCases := []koboGetTestCase{
		{
			name:    "full sync with unread and archived",
			reqBody: &models.KoboGetRequest{Count: "10", AccessToken: mockDeviceToken}, // No 'Since'
			mockBookmarksSync: []readeck.BookmarkSync{
				{ID: "1", Type: "update"},
				{ID: "2", Type: "update"},
			},
			mockBookmarkDetails: map[string]*readeck.Bookmark{
				"1": {ID: "1", Title: "Unread", IsArchived: false},
				"2": {ID: "2", Title: "Archived", IsArchived: true},
			},
			expectedStatus:   http.StatusOK,
			expectedListSize: 1, // Only the unread item
			expectedTotal:    1,
		},
		{
			name:    "full sync with favorited item",
			reqBody: &models.KoboGetRequest{Count: "10", AccessToken: mockDeviceToken}, // No 'Since'
			mockBookmarksSync: []readeck.BookmarkSync{
				{ID: "1", Type: "update"},
			},
			mockBookmarkDetails: map[string]*readeck.Bookmark{
				"1": {ID: "1", Title: "Favorited", IsArchived: false, IsMarked: true},
			},
			expectedStatus:   http.StatusOK,
			expectedListSize: 1,
			expectedTotal:    1,
		},
		{
			name:    "full sync with image item",
			reqBody: &models.KoboGetRequest{Count: "10", AccessToken: mockDeviceToken}, // No 'Since'
			mockBookmarksSync: []readeck.BookmarkSync{
				{ID: "1", Type: "update"},
			},
			mockBookmarkDetails: map[string]*readeck.Bookmark{
				"1": {
					ID:         "1",
					Title:      "Item With Image",
					IsArchived: false,
					Resources: readeck.Resources{
						Image: &readeck.ResourceImage{
							Src: "http://example.com/image.png",
						},
					},
				},
			},
			expectedStatus:   http.StatusOK,
			expectedListSize: 1,
			expectedTotal:    1,
		},
		{
			name:    "incremental sync with deleted",
			reqBody: &models.KoboGetRequest{Since: sinceValue, AccessToken: mockDeviceToken},
			mockBookmarksSync: []readeck.BookmarkSync{
				{ID: "1", Type: "delete"},
			},
			mockBookmarkDetails: map[string]*readeck.Bookmark{},
			expectedStatus:      http.StatusOK,
			expectedListSize:    1, // The deleted status update
			expectedTotal:       0,
		},
		{
			name:    "incremental sync with newly archived",
			reqBody: &models.KoboGetRequest{Since: sinceValue, AccessToken: mockDeviceToken},
			mockBookmarksSync: []readeck.BookmarkSync{
				{ID: "1", Type: "update"},
			},
			mockBookmarkDetails: map[string]*readeck.Bookmark{
				"1": {ID: "1", Title: "Newly Archived", IsArchived: true},
			},
			expectedStatus:   http.StatusOK,
			expectedListSize: 1, // The full archived item
			expectedTotal:    0,
		},
		{
			name:                 "incremental sync with GetBookmarksSync error",
			reqBody:              &models.KoboGetRequest{Since: sinceValue, AccessToken: mockDeviceToken},
			mockBookmarksSyncErr: fmt.Errorf("sync error"),
			expectedStatus:       http.StatusInternalServerError,
		},
		{
			name:    "incremental sync with SyncBookmarksContent error",
			reqBody: &models.KoboGetRequest{Since: sinceValue, AccessToken: mockDeviceToken},
			mockBookmarksSync: []readeck.BookmarkSync{
				{ID: "1", Type: "update"},
			},
			mockBookmarkDetailsErr: fmt.Errorf("details error"),
			expectedStatus:         http.StatusInternalServerError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Common mock HTTP client func
			mockHTTPClientFunc := func(w http.ResponseWriter, r *http.Request) {
				if tc.mockBookmarksSyncErr != nil && r.URL.Path == "/api/bookmarks/sync" && r.Method == http.MethodGet {
					http.Error(w, tc.mockBookmarksSyncErr.Error(), http.StatusInternalServerError)
					return
				}
				if tc.mockBookmarkDetailsErr != nil && r.URL.Path == "/api/bookmarks/sync" && r.Method == http.MethodPost {
					http.Error(w, tc.mockBookmarkDetailsErr.Error(), http.StatusInternalServerError)
					return
				}

				switch r.URL.Path {
				case "/api/bookmarks/sync":
					switch r.Method {
					case http.MethodGet:
						jsonBytes, _ := json.Marshal(tc.mockBookmarksSync)
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusOK)
						_, _ = w.Write(jsonBytes)
					case http.MethodPost:
						boundary := "MULTIPART_BOUNDARY"
						var b bytes.Buffer
						writer := multipart.NewWriter(&b)
						if err := writer.SetBoundary(boundary); err != nil {
							t.Fatalf("Failed to set boundary: %v", err)
						}
						reqBodyBytes, _ := io.ReadAll(r.Body)
						var syncRequest struct {
							IDs []string `json:"id"`
						}
						if err := json.Unmarshal(reqBodyBytes, &syncRequest); err != nil {
							t.Fatalf("Failed to unmarshal sync request: %v", err)
						}
						for _, id := range syncRequest.IDs {
							if bm, ok := tc.mockBookmarkDetails[id]; ok && bm != nil {
								partHeader := make(textproto.MIMEHeader)
								partHeader.Set("Content-Type", "application/json")
								partHeader.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="bookmark_%s.json"`, id))
								part, _ := writer.CreatePart(partHeader)
								_ = json.NewEncoder(part).Encode(bm)
							}
						}
						_ = writer.Close()
						w.Header().Set("Content-Type", fmt.Sprintf("multipart/mixed; boundary=%s", boundary))
						w.WriteHeader(http.StatusOK)
						_, _ = w.Write(b.Bytes())
					}
				default:
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{}`))
				}
			}

			mockServer := httptest.NewServer(http.HandlerFunc(mockHTTPClientFunc))
			defer mockServer.Close()

			app := NewApp(
				WithConfig(&config.Config{
					Users:   []config.User{{Token: mockDeviceToken, ReadeckAccessToken: mockPlaintextReadeckToken}},
					Readeck: config.ConfigReadeck{Host: mockServer.URL},
				}),
				WithLogger(testLogger),
			)

			jsonBody, _ := json.Marshal(tc.reqBody)
			req := httptest.NewRequest(http.MethodPost, "/api/kobo/get", bytes.NewReader(jsonBody))
			rr := httptest.NewRecorder()

			readeckClient, err := readeck.NewClient(mockServer.URL, "test-token", testLogger, mockServer.Client())
			if err != nil {
				t.Fatalf("Failed to create readeck client: %v", err)
			}

			var resultList map[string]models.KoboArticleItem
			var total int
			var syncErr error

			if tc.reqBody.Since == nil {
				resultList, total, syncErr = app.handleFullSync(req.Context(), readeckClient, tc.reqBody)
			} else {
				var since time.Time
				if s, ok := tc.reqBody.Since.(float64); ok {
					since = time.Unix(int64(s), 0)
				}
				resultList, total, syncErr = app.handleIncrementalSync(req.Context(), readeckClient, &since)
			}

			if syncErr != nil {
				if tc.expectedStatus != http.StatusInternalServerError {
					t.Errorf("expected status %d on sync error, but test case expected %d", http.StatusInternalServerError, tc.expectedStatus)
				}
				// Simulate the controller writing the error
				http.Error(rr, syncErr.Error(), http.StatusInternalServerError)
			} else {
				resp := models.KoboGetResponse{
					Status: 1,
					List:   resultList,
					Total:  total,
				}
				rr.Header().Set("Content-Type", "application/json")
				rr.WriteHeader(http.StatusOK)
				if err := json.NewEncoder(rr).Encode(resp); err != nil {
					t.Fatalf("Failed to encode response: %v", err)
				}
			}

			// Assertions
			if rr.Code != tc.expectedStatus {
				t.Errorf("expected status %d, got %d. Body: %s", tc.expectedStatus, rr.Code, rr.Body.String())
				return
			}

			if tc.expectedStatus == http.StatusOK {
				var resp models.KoboGetResponse
				if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if len(resp.List) != tc.expectedListSize {
					t.Errorf("expected %d item(s) in list, got %d", tc.expectedListSize, len(resp.List))
				}
				if resp.Total != tc.expectedTotal {
					t.Errorf("expected total to be %d, got %d", tc.expectedTotal, resp.Total)
				}

				// Specific checks for each test case
				switch tc.name {
				case "full sync with favorited item":
					item := resp.List["1"]
					if item.Favorite != "1" {
						t.Errorf("expected favorited item 'favorite' status to be '1', got '%s'", item.Favorite)
					}
				case "incremental sync with deleted":
					item := resp.List["1"]
					if item.Status != "2" {
						t.Errorf("expected deleted item status to be '2', got '%s'", item.Status)
					}
				case "incremental sync with newly archived":
					item := resp.List["1"]
					if item.Status != "1" {
						t.Errorf("expected archived item status to be '1', got '%s'", item.Status)
					}
				case "full sync with image item":
					item := resp.List["1"]
					if item.HasImage != "1" {
						t.Errorf("expected has_image to be '1', got '%s'", item.HasImage)
					}
					if item.Image.Src != "http://example.com/image.png" {
						t.Errorf("expected image.src to be 'http://example.com/image.png', got '%s'", item.Image.Src)
					}
				}
			}
		})
	}
}
// koboDownloadTestCase defines the structure for test cases in TestHandleKoboDownload.
type koboDownloadTestCase struct {
	name           string
	reqBody        any // Can be JSON or form data
	contentType    string
	expectedStatus int
	mockBookmarks  []readeck.Bookmark
	mockArticle    string
}

func TestHandleKoboDownload(t *testing.T) {
	testCases := []koboDownloadTestCase{
		{
			name: "successful download (JSON)",
			reqBody: models.KoboDownloadRequest{
				AccessToken: mockDeviceToken,
				URL:         "http://example.com/article1",
			},
			contentType:    "application/json",
			expectedStatus: http.StatusOK,
			mockBookmarks: []readeck.Bookmark{
				{ID: "1", Title: "Test Article", URL: "http://example.com/article1"},
			},
			mockArticle: `<html><body><h1>Test Article</h1><img src="http://example.com/image.png"></body></html>`,
		},
		{
			name: "successful download (Form)",
			reqBody: url.Values{
				"access_token": {mockDeviceToken},
				"url":          {"http://example.com/article1"},
			},
			contentType:    "application/x-www-form-urlencoded",
			expectedStatus: http.StatusOK,
			mockBookmarks: []readeck.Bookmark{
				{ID: "1", Title: "Test Article", URL: "http://example.com/article1"},
			},
			mockArticle: `<html><body><h1>Test Article</h1><img src="http://example.com/image.png"></body></html>`,
		},
		{
			name: "missing url",
			reqBody: models.KoboDownloadRequest{
				AccessToken: mockDeviceToken,
				URL:         "",
			},
			contentType:    "application/json",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "invalid access token",
			reqBody: models.KoboDownloadRequest{
				AccessToken: "invalid-device-token",
				URL:         "http://example.com/article1",
			},
			contentType:    "application/json",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/bookmarks" {
					jsonBytes, _ := json.Marshal(tc.mockBookmarks)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(jsonBytes)
					return
				}
				if strings.HasSuffix(r.URL.Path, "/article") {
					w.Header().Set("Content-Type", "text/html")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(tc.mockArticle))
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			defer mockServer.Close()

			app := NewApp(
				WithConfig(&config.Config{
					Users: []config.User{
						{
							Token:              mockDeviceToken,
							ReadeckAccessToken: mockPlaintextReadeckToken,
						},
					},
					Readeck: config.ConfigReadeck{Host: mockServer.URL},
				}),
				WithLogger(testLogger),
				WithReadeckHTTPClient(mockServer.Client()),
			)

			var body io.Reader
			switch tc.contentType {
			case "application/json":
				jsonBody, err := json.Marshal(tc.reqBody)
				if err != nil {
					t.Fatalf("Failed to marshal request body: %v", err)
				}
				body = bytes.NewReader(jsonBody)
			case "application/x-www-form-urlencoded":
				formValues := tc.reqBody.(url.Values)
				body = strings.NewReader(formValues.Encode())
			}

			req := httptest.NewRequest(http.MethodPost, "/api/kobo/download", body)
			req.Header.Add("Content-Type", tc.contentType)
			rr := httptest.NewRecorder()

			app.HandleKoboDownload(rr, req)

			if rr.Code != tc.expectedStatus {
				t.Errorf("expected status %d, got %d", tc.expectedStatus, rr.Code)
			}

			if tc.expectedStatus == http.StatusOK {
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
		})
	}
}

// koboSendTestCase defines the structure for test cases in TestHandleKoboSend.
type koboSendTestCase struct {
	name                string
	actions             []any
	accessToken         string
	expectedStatus      bool
	expectedResults     []bool
	expectedUpdatedID   string
	expectedUpdatedData map[string]any
	expectedCreatedURL  string
	expectedHTTPStatus  int
}

func TestHandleKoboSend(t *testing.T) {
	var updatedBookmarkID string
	var updatedBookmarkData map[string]any
	var createdBookmarkURL string

	testCases := []koboSendTestCase{
		{
			name: "archive action",
			actions: []any{
				map[string]any{"action": "archive", "item_id": "1"},
			},
			accessToken:         mockDeviceToken,
			expectedStatus:      true,
			expectedResults:     []bool{true},
			expectedUpdatedID:   "1",
			expectedUpdatedData: map[string]any{"is_archived": true},
			expectedHTTPStatus:  http.StatusOK,
		},
		{
			name: "readd action",
			actions: []any{
				map[string]any{"action": "readd", "item_id": "2"},
			},
			accessToken:         mockDeviceToken,
			expectedStatus:      true,
			expectedResults:     []bool{true},
			expectedUpdatedID:   "2",
			expectedUpdatedData: map[string]any{"is_archived": false},
			expectedHTTPStatus:  http.StatusOK,
		},
		{
			name: "favorite action",
			actions: []any{
				map[string]any{"action": "favorite", "item_id": "3"},
			},
			accessToken:         mockDeviceToken,
			expectedStatus:      true,
			expectedResults:     []bool{true},
			expectedUpdatedID:   "3",
			expectedUpdatedData: map[string]any{"is_marked": true},
			expectedHTTPStatus:  http.StatusOK,
		},
		{
			name: "unfavorite action",
			actions: []any{
				map[string]any{"action": "unfavorite", "item_id": "4"},
			},
			accessToken:         mockDeviceToken,
			expectedStatus:      true,
			expectedResults:     []bool{true},
			expectedUpdatedID:   "4",
			expectedUpdatedData: map[string]any{"is_marked": false},
			expectedHTTPStatus:  http.StatusOK,
		},
		{
			name: "delete action",
			actions: []any{
				map[string]any{"action": "delete", "item_id": "5"},
			},
			accessToken:         mockDeviceToken,
			expectedStatus:      true,
			expectedResults:     []bool{true},
			expectedUpdatedID:   "5",
			expectedUpdatedData: map[string]any{"is_deleted": true},
			expectedHTTPStatus:  http.StatusOK,
		},
		{
			name: "add action",
			actions: []any{
				map[string]any{"action": "add", "url": "http://example.com/new"},
			},
			accessToken:        mockDeviceToken,
			expectedStatus:     true,
			expectedResults:    []bool{true},
			expectedCreatedURL: "http://example.com/new",
			expectedHTTPStatus: http.StatusOK,
		},
		{
			name: "unknown action",
			actions: []any{
				map[string]any{"action": "unknown", "item_id": "6"},
			},
			accessToken:        mockDeviceToken,
			expectedStatus:     false,
			expectedResults:    []bool{false},
			expectedHTTPStatus: http.StatusOK,
		},
		{
			name: "invalid action",
			actions: []any{
				"invalid action",
			},
			accessToken:        mockDeviceToken,
			expectedStatus:     false,
			expectedResults:    []bool{false},
			expectedHTTPStatus: http.StatusOK,
		},
		{
			name: "invalid access token",
			actions: []any{
				map[string]any{"action": "archive", "item_id": "1"},
			},
			accessToken:        "invalid-device-token",
			expectedStatus:     false,
			expectedResults:    []bool{},
			expectedHTTPStatus: http.StatusUnauthorized,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset mock data
			updatedBookmarkID = ""
			updatedBookmarkData = nil
			createdBookmarkURL = ""

			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPatch {
					updatedBookmarkID = strings.TrimPrefix(r.URL.Path, "/api/bookmarks/")
					bodyBytes, _ := io.ReadAll(r.Body)
					if err := json.Unmarshal(bodyBytes, &updatedBookmarkData); err != nil {
						t.Fatalf("Failed to unmarshal: %v", err)
					}
				}
				if r.Method == http.MethodPost {
					var data struct {
						URL string `json:"url"`
					}
					bodyBytes, _ := io.ReadAll(r.Body)
					if err := json.Unmarshal(bodyBytes, &data); err != nil {
						t.Fatalf("Failed to unmarshal: %v", err)
					}
					createdBookmarkURL = data.URL
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"status": "ok"}`))
			}))
			defer mockServer.Close()

			app := NewApp(
				WithConfig(&config.Config{
					Users: []config.User{
						{
							Token:              mockDeviceToken,
							ReadeckAccessToken: mockPlaintextReadeckToken,
						},
					},
					Readeck: config.ConfigReadeck{Host: mockServer.URL},
				}),
				WithLogger(testLogger),
				WithReadeckHTTPClient(mockServer.Client()),
			)

			reqBody := models.KoboSendRequest{AccessToken: tc.accessToken, Actions: tc.actions}
			body, err := json.Marshal(reqBody)
			if err != nil {
				t.Fatalf("Failed to marshal request body: %v", err)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/kobo/send", bytes.NewReader(body))
			rr := httptest.NewRecorder()

			app.HandleKoboSend(rr, req)

			if rr.Code != tc.expectedHTTPStatus {
				t.Errorf("expected status %d, got %d", tc.expectedHTTPStatus, rr.Code)
			}

			if tc.expectedHTTPStatus == http.StatusOK {
				var resp map[string]any
				if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}

				if status, _ := resp["status"].(bool); status != tc.expectedStatus {
					t.Errorf("expected status %v, got %v", tc.expectedStatus, status)
				}

				results, ok := resp["action_results"].([]any)
				if !ok && len(tc.expectedResults) > 0 { // Only check if expectedResults is not empty
					t.Fatalf("expected action_results to be a slice, got %T", resp["action_results"])
				}
				if len(results) != len(tc.expectedResults) {
					t.Fatalf("expected action_results to be a slice of length %d, got %d", len(tc.expectedResults), len(results))
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
			}
		})
	}
}

func TestHandleConvertImage(t *testing.T) {
	testLogger := logger.New(logger.DEBUG)

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
		app := NewApp(WithConfig(&config.Config{}), WithLogger(testLogger))
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
		app := NewApp(WithConfig(&config.Config{}), WithLogger(testLogger))
		req := httptest.NewRequest(http.MethodGet, "/api/convert-image", nil)
		rr := httptest.NewRecorder()

		app.HandleConvertImage(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
		}
	})

	t.Run("image fetch failed", func(t *testing.T) {
		// Create a mock HTTP client that immediately returns an error
		mockRT := &MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				return nil, fmt.Errorf("mock network error")
			},
		}
		mockClient := &http.Client{Transport: mockRT}

		app := NewApp(WithConfig(&config.Config{}), WithLogger(testLogger), WithImageHTTPClient(mockClient))
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

		app := NewApp(WithConfig(&config.Config{}), WithLogger(testLogger))
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


