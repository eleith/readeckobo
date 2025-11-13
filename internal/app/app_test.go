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

// koboGetTestCase defines the structure for test cases in TestHandleKoboGet.

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
	mockHTTPClientFunc     func(t *testing.T, tc *koboGetTestCase) *http.Client
}

func TestHandleKoboGet(t *testing.T) {
	testCases := []koboGetTestCase{
		{
			name:    "successful get",
			reqBody: &models.KoboGetRequest{Count: "1", AccessToken: mockDeviceToken},
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
			mockHTTPClientFunc: func(t *testing.T, tc *koboGetTestCase) *http.Client {
				return &http.Client{
					Transport: &MockRoundTripper{
						RoundTripFunc: func(req *http.Request) (*http.Response, error) {
							switch req.URL.Path {
							case "/api/bookmarks/sync":
								switch req.Method {
								case http.MethodGet:
									jsonBytes, _ := json.Marshal(tc.mockBookmarksSync)
									return &http.Response{
										StatusCode: http.StatusOK,
										Body:       io.NopCloser(bytes.NewReader(jsonBytes)),
										Header:     make(http.Header),
									}, nil
								case http.MethodPost:
									boundary := "MULTIPART_BOUNDARY"
									var b bytes.Buffer
									writer := multipart.NewWriter(&b)
									if err := writer.SetBoundary(boundary); err != nil {
										t.Fatalf("Failed to set boundary: %v", err)
									}

									reqBodyBytes, _ := io.ReadAll(req.Body)
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
											part, err := writer.CreatePart(partHeader)
											if err != nil {
												t.Fatalf("Failed to create part: %v", err)
											}
											if err := json.NewEncoder(part).Encode(bm); err != nil {
												t.Fatalf("Failed to encode bookmark: %v", err)
											}
										}
									}
									if err := writer.Close(); err != nil {
										t.Fatalf("Failed to close writer: %v", err)
									}

									header := make(http.Header)
									header.Set("Content-Type", fmt.Sprintf("multipart/mixed; boundary=%s", boundary))
									return &http.Response{
										StatusCode: http.StatusOK,
										Body:       io.NopCloser(bytes.NewReader(b.Bytes())),
										Header:     header,
									}, nil
								}
							}
							return &http.Response{
								StatusCode: http.StatusOK,
								Body:       io.NopCloser(strings.NewReader(`{"status": "ok"}`)),
								Header:     make(http.Header),
							}, nil
						},
					},
				}
			},
		},
		{
			name:    "delete sync type",
			reqBody: &models.KoboGetRequest{Count: "1", AccessToken: mockDeviceToken},
			mockBookmarksSync: []readeck.BookmarkSync{
				{ID: "1", Type: "delete"},
			},
			expectedStatus:   http.StatusOK,
			expectedListSize: 1,
			expectedTotal:    0,
			mockHTTPClientFunc: func(t *testing.T, tc *koboGetTestCase) *http.Client {
				return &http.Client{
					Transport: &MockRoundTripper{
						RoundTripFunc: func(req *http.Request) (*http.Response, error) {
							switch req.URL.Path {
							case "/api/bookmarks/sync":
								switch req.Method {
								case http.MethodGet:
									jsonBytes, _ := json.Marshal(tc.mockBookmarksSync)
									return &http.Response{
										StatusCode: http.StatusOK,
										Body:       io.NopCloser(bytes.NewReader(jsonBytes)),
										Header:     make(http.Header),
									}, nil
								case http.MethodPost:
									// For delete sync type, no bookmark details are returned
									boundary := "MULTIPART_BOUNDARY"
									var b bytes.Buffer
									writer := multipart.NewWriter(&b)
									if err := writer.SetBoundary(boundary); err != nil {
										t.Fatalf("Failed to set boundary: %v", err)
									}
									if err := writer.Close(); err != nil {
										t.Fatalf("Failed to close writer: %v", err)
									}

									header := make(http.Header)
									header.Set("Content-Type", fmt.Sprintf("multipart/mixed; boundary=%s", boundary))
									return &http.Response{
										StatusCode: http.StatusOK,
										Body:       io.NopCloser(bytes.NewReader(b.Bytes())),
										Header:     header,
									}, nil
								}
							}
							return &http.Response{
								StatusCode: http.StatusOK,
								Body:       io.NopCloser(strings.NewReader(`{"status": "ok"}`)),
								Header:     make(http.Header),
							}, nil
						},
					},
				}
			},
		},
		{
			name:    "get bookmark details error",
			reqBody: &models.KoboGetRequest{Count: "1", AccessToken: mockDeviceToken},
			mockBookmarksSync: []readeck.BookmarkSync{
				{ID: "1", Type: "update"},
			},
			mockBookmarkDetailsErr: fmt.Errorf("details error"),
			expectedStatus:         http.StatusInternalServerError,
			expectedListSize:       0,
			expectedTotal:          0,
			mockHTTPClientFunc: func(t *testing.T, tc *koboGetTestCase) *http.Client {
				return &http.Client{
					Transport: &MockRoundTripper{
						RoundTripFunc: func(req *http.Request) (*http.Response, error) {
							switch req.URL.Path {
							case "/api/bookmarks/sync":
								switch req.Method {
								case http.MethodGet:
									jsonBytes, _ := json.Marshal(tc.mockBookmarksSync)
									return &http.Response{
										StatusCode: http.StatusOK,
										Body:       io.NopCloser(bytes.NewReader(jsonBytes)),
										Header:     make(http.Header),
									}, nil
								case http.MethodPost:
									return nil, tc.mockBookmarkDetailsErr // Simulate error from Readeck API
								}
							}
							return &http.Response{
								StatusCode: http.StatusOK,
								Body:       io.NopCloser(strings.NewReader(`{"status": "ok"}`)),
								Header:     make(http.Header),
							}, nil
						},
					},
				}
			},
		},
		{
			name:    "get bookmark details nil",
			reqBody: &models.KoboGetRequest{Count: "1", AccessToken: mockDeviceToken},
			mockBookmarksSync: []readeck.BookmarkSync{
				{ID: "1", Type: "update"},
			},
			mockBookmarkDetails: map[string]*readeck.Bookmark{
				"1": nil,
			},
			expectedStatus:   http.StatusOK,
			expectedListSize: 0,
			expectedTotal:    0,
			mockHTTPClientFunc: func(t *testing.T, tc *koboGetTestCase) *http.Client {
				return &http.Client{
					Transport: &MockRoundTripper{
						RoundTripFunc: func(req *http.Request) (*http.Response, error) {
							switch req.URL.Path {
							case "/api/bookmarks/sync":
								switch req.Method {
								case http.MethodGet:
									jsonBytes, _ := json.Marshal(tc.mockBookmarksSync)
									return &http.Response{
										StatusCode: http.StatusOK,
										Body:       io.NopCloser(bytes.NewReader(jsonBytes)),
										Header:     make(http.Header),
									}, nil
								case http.MethodPost:
									boundary := "MULTIPART_BOUNDARY"
									var b bytes.Buffer
									writer := multipart.NewWriter(&b)
									if err := writer.SetBoundary(boundary); err != nil {
										t.Fatalf("Failed to set boundary: %v", err)
									}

									reqBodyBytes, _ := io.ReadAll(req.Body)
									var syncRequest struct {
										IDs []string `json:"id"`
									}
									if err := json.Unmarshal(reqBodyBytes, &syncRequest); err != nil {
										t.Fatalf("Failed to unmarshal sync request: %v", err)
									}

									for _, id := range syncRequest.IDs {
										// Do not add part if bookmark details are nil
										if bm, ok := tc.mockBookmarkDetails[id]; ok && bm != nil {
											partHeader := make(textproto.MIMEHeader)
											partHeader.Set("Content-Type", "application/json")
											partHeader.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="bookmark_%s.json"`, id))
											part, err := writer.CreatePart(partHeader)
											if err != nil {
												t.Fatalf("Failed to create part: %v", err)
											}
											if err := json.NewEncoder(part).Encode(bm); err != nil {
												t.Fatalf("Failed to encode bookmark: %v", err)
											}
										}
									}
									if err := writer.Close(); err != nil {
										t.Fatalf("Failed to close writer: %v", err)
									}

									header := make(http.Header)
									header.Set("Content-Type", fmt.Sprintf("multipart/mixed; boundary=%s", boundary))
									return &http.Response{
										StatusCode: http.StatusOK,
										Body:       io.NopCloser(bytes.NewReader(b.Bytes())),
										Header:     header,
									}, nil
								}
							}
							return &http.Response{
								StatusCode: http.StatusOK,
								Body:       io.NopCloser(strings.NewReader(`{"status": "ok"}`)),
								Header:     make(http.Header),
							}, nil
						},
					},
				}
			},
		},
		{
			name:                 "get bookmarks sync error",
			reqBody:              &models.KoboGetRequest{Count: "1", AccessToken: mockDeviceToken},
			mockBookmarksSyncErr: fmt.Errorf("sync error"),
			expectedStatus:       http.StatusInternalServerError,
			mockHTTPClientFunc: func(t *testing.T, tc *koboGetTestCase) *http.Client {
				return &http.Client{
					Transport: &MockRoundTripper{
						RoundTripFunc: func(req *http.Request) (*http.Response, error) {
							switch req.URL.Path {
							case "/api/bookmarks/sync":
								if req.Method == http.MethodGet {
									return nil, tc.mockBookmarksSyncErr // Simulate error from Readeck API
								}
							}
							return &http.Response{
								StatusCode: http.StatusOK,
								Body:       io.NopCloser(strings.NewReader(`{"status": "ok"}`)),
								Header:     make(http.Header),
							}, nil
						},
					},
				}
			},
		},
		{
			name:           "invalid request body",
			reqBody:        nil,
			expectedStatus: http.StatusBadRequest,
			mockHTTPClientFunc: func(t *testing.T, tc *koboGetTestCase) *http.Client {
				return &http.Client{
					Transport: &MockRoundTripper{
						RoundTripFunc: func(req *http.Request) (*http.Response, error) {
							return &http.Response{
								StatusCode: http.StatusOK,
								Body:       io.NopCloser(strings.NewReader(`{"status": "ok"}`)),
								Header:     make(http.Header),
							}, nil
						},
					},
				}
			},
		},
		{
			name:           "invalid access token",
			reqBody:        &models.KoboGetRequest{Count: "1", AccessToken: "invalid-device-token"},
			expectedStatus: http.StatusUnauthorized,
			mockHTTPClientFunc: func(t *testing.T, tc *koboGetTestCase) *http.Client {
				return &http.Client{
					Transport: &MockRoundTripper{
						RoundTripFunc: func(req *http.Request) (*http.Response, error) {
							return &http.Response{
								StatusCode: http.StatusOK,
								Body:       io.NopCloser(strings.NewReader(`{"status": "ok"}`)),
								Header:     make(http.Header),
							}, nil
						},
					},
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			app := NewApp(
				WithConfig(&config.Config{
					Users: []config.User{
						{
							Token:              mockDeviceToken,
							ReadeckAccessToken: mockPlaintextReadeckToken,
						},
					},
					Readeck: config.ConfigReadeck{Host: "http://mock-readeck.com"},
				}),
				WithLogger(testLogger),
				WithReadeckHTTPClient(tc.mockHTTPClientFunc(t, &tc)), // Pass the dynamically created mock HTTP client
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
				var resp models.KoboGetResponse
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

// koboDownloadTestCase defines the structure for test cases in TestHandleKoboDownload.
type koboDownloadTestCase struct {
	name               string
	reqBody            any // Can be JSON or form data
	contentType        string
	expectedStatus     int
	mockBookmarks      []readeck.Bookmark
	mockArticle        string
	mockHTTPClientFunc func(t *testing.T, tc *koboDownloadTestCase) *http.Client
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
			mockHTTPClientFunc: func(t *testing.T, tc *koboDownloadTestCase) *http.Client {
				return &http.Client{
					Transport: &MockRoundTripper{
						RoundTripFunc: func(req *http.Request) (*http.Response, error) {
							if req.URL.Path == "/api/bookmarks" {
								jsonBytes, _ := json.Marshal(tc.mockBookmarks)
								return &http.Response{
									StatusCode: http.StatusOK,
									Body:       io.NopCloser(bytes.NewReader(jsonBytes)),
									Header:     make(http.Header),
								}, nil
							}
							if strings.HasSuffix(req.URL.Path, "/article") {
								return &http.Response{
									StatusCode: http.StatusOK,
									Body:       io.NopCloser(strings.NewReader(tc.mockArticle)),
									Header:     make(http.Header),
								}, nil
							}
							// Mock image server for /api/convert-image
							if strings.Contains(req.URL.Path, "/api/convert-image") {
								return &http.Response{
									StatusCode: http.StatusOK,
									Body:       io.NopCloser(bytes.NewReader([]byte("mock image data"))),
									Header:     make(http.Header),
								}, nil
							}
							return &http.Response{
								StatusCode: http.StatusOK,
								Body:       io.NopCloser(strings.NewReader(`{"status": "ok"}`)),
								Header:     make(http.Header),
							}, nil
						},
					},
				}
			},
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
			mockHTTPClientFunc: func(t *testing.T, tc *koboDownloadTestCase) *http.Client {
				return &http.Client{
					Transport: &MockRoundTripper{
						RoundTripFunc: func(req *http.Request) (*http.Response, error) {
							if req.URL.Path == "/api/bookmarks" {
								jsonBytes, _ := json.Marshal(tc.mockBookmarks)
								return &http.Response{
									StatusCode: http.StatusOK,
									Body:       io.NopCloser(bytes.NewReader(jsonBytes)),
									Header:     make(http.Header),
								}, nil
							}
							if strings.HasSuffix(req.URL.Path, "/article") {
								return &http.Response{
									StatusCode: http.StatusOK,
									Body:       io.NopCloser(strings.NewReader(tc.mockArticle)),
									Header:     make(http.Header),
								}, nil
							}
							// Mock image server for /api/convert-image
							if strings.Contains(req.URL.Path, "/api/convert-image") {
								return &http.Response{
									StatusCode: http.StatusOK,
									Body:       io.NopCloser(bytes.NewReader([]byte("mock image data"))),
									Header:     make(http.Header),
								}, nil
							}
							return &http.Response{
								StatusCode: http.StatusOK,
								Body:       io.NopCloser(strings.NewReader(`{"status": "ok"}`)),
								Header:     make(http.Header),
							}, nil
						},
					},
				}
			},
		},
		{
			name: "missing url",
			reqBody: models.KoboDownloadRequest{
				AccessToken: mockDeviceToken,
				URL:         "",
			},
			contentType:    "application/json",
			expectedStatus: http.StatusBadRequest,
			mockHTTPClientFunc: func(t *testing.T, tc *koboDownloadTestCase) *http.Client {
				return &http.Client{
					Transport: &MockRoundTripper{
						RoundTripFunc: func(req *http.Request) (*http.Response, error) {
							return &http.Response{
								StatusCode: http.StatusOK,
								Body:       io.NopCloser(strings.NewReader(`{"status": "ok"}`)),
								Header:     make(http.Header),
							}, nil
						},
					},
				}
			},
		},
		{
			name: "invalid access token",
			reqBody: models.KoboDownloadRequest{
				AccessToken: "invalid-device-token",
				URL:         "http://example.com/article1",
			},
			contentType:    "application/json",
			expectedStatus: http.StatusUnauthorized,
			mockHTTPClientFunc: func(t *testing.T, tc *koboDownloadTestCase) *http.Client {
				return &http.Client{
					Transport: &MockRoundTripper{
						RoundTripFunc: func(req *http.Request) (*http.Response, error) {
							return &http.Response{
								StatusCode: http.StatusOK,
								Body:       io.NopCloser(strings.NewReader(`{"status": "ok"}`)),
								Header:     make(http.Header),
							}, nil
						},
					},
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			app := NewApp(
				WithConfig(&config.Config{
					Users: []config.User{
						{
							Token:              mockDeviceToken,
							ReadeckAccessToken: mockPlaintextReadeckToken,
						},
					},
					Readeck: config.ConfigReadeck{Host: "http://mock-readeck.com"},
				}),
				WithLogger(testLogger),
				WithReadeckHTTPClient(tc.mockHTTPClientFunc(t, &tc)),
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
	mockHTTPClientFunc  func(t *testing.T, tc *koboSendTestCase, updatedBookmarkID *string, updatedBookmarkData *map[string]any, createdBookmarkURL *string) *http.Client
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
			mockHTTPClientFunc: func(t *testing.T, tc *koboSendTestCase, updatedBookmarkID *string, updatedBookmarkData *map[string]any, createdBookmarkURL *string) *http.Client {
				return &http.Client{
					Transport: &MockRoundTripper{
						RoundTripFunc: func(req *http.Request) (*http.Response, error) {
							if req.Method == http.MethodPatch {
								*updatedBookmarkID = strings.TrimPrefix(req.URL.Path, "/api/bookmarks/")
								bodyBytes, _ := io.ReadAll(req.Body)
								if err := json.Unmarshal(bodyBytes, updatedBookmarkData); err != nil {
									t.Fatalf("Failed to unmarshal: %v", err)
								}
							}
							return &http.Response{
								StatusCode: http.StatusOK,
								Body:       io.NopCloser(strings.NewReader(`{"status": "ok"}`)),
								Header:     make(http.Header),
							}, nil
						},
					},
				}
			},
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
			mockHTTPClientFunc: func(t *testing.T, tc *koboSendTestCase, updatedBookmarkID *string, updatedBookmarkData *map[string]any, createdBookmarkURL *string) *http.Client {
				return &http.Client{
					Transport: &MockRoundTripper{
						RoundTripFunc: func(req *http.Request) (*http.Response, error) {
							if req.Method == http.MethodPatch {
								*updatedBookmarkID = strings.TrimPrefix(req.URL.Path, "/api/bookmarks/")
								bodyBytes, _ := io.ReadAll(req.Body)
								if err := json.Unmarshal(bodyBytes, updatedBookmarkData); err != nil {
									t.Fatalf("Failed to unmarshal: %v", err)
								}
							}
							return &http.Response{
								StatusCode: http.StatusOK,
								Body:       io.NopCloser(strings.NewReader(`{"status": "ok"}`)),
								Header:     make(http.Header),
							}, nil
						},
					},
				}
			},
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
			mockHTTPClientFunc: func(t *testing.T, tc *koboSendTestCase, updatedBookmarkID *string, updatedBookmarkData *map[string]any, createdBookmarkURL *string) *http.Client {
				return &http.Client{
					Transport: &MockRoundTripper{
						RoundTripFunc: func(req *http.Request) (*http.Response, error) {
							if req.Method == http.MethodPatch {
								*updatedBookmarkID = strings.TrimPrefix(req.URL.Path, "/api/bookmarks/")
								bodyBytes, _ := io.ReadAll(req.Body)
								if err := json.Unmarshal(bodyBytes, updatedBookmarkData); err != nil {
									t.Fatalf("Failed to unmarshal: %v", err)
								}
							}
							return &http.Response{
								StatusCode: http.StatusOK,
								Body:       io.NopCloser(strings.NewReader(`{"status": "ok"}`)),
								Header:     make(http.Header),
							}, nil
						},
					},
				}
			},
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
			mockHTTPClientFunc: func(t *testing.T, tc *koboSendTestCase, updatedBookmarkID *string, updatedBookmarkData *map[string]any, createdBookmarkURL *string) *http.Client {
				return &http.Client{
					Transport: &MockRoundTripper{
						RoundTripFunc: func(req *http.Request) (*http.Response, error) {
							if req.Method == http.MethodPatch {
								*updatedBookmarkID = strings.TrimPrefix(req.URL.Path, "/api/bookmarks/")
								bodyBytes, _ := io.ReadAll(req.Body)
								if err := json.Unmarshal(bodyBytes, updatedBookmarkData); err != nil {
									t.Fatalf("Failed to unmarshal: %v", err)
								}
							}
							return &http.Response{
								StatusCode: http.StatusOK,
								Body:       io.NopCloser(strings.NewReader(`{"status": "ok"}`)),
								Header:     make(http.Header),
							}, nil
						},
					},
				}
			},
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
			mockHTTPClientFunc: func(t *testing.T, tc *koboSendTestCase, updatedBookmarkID *string, updatedBookmarkData *map[string]any, createdBookmarkURL *string) *http.Client {
				return &http.Client{
					Transport: &MockRoundTripper{
						RoundTripFunc: func(req *http.Request) (*http.Response, error) {
							if req.Method == http.MethodPatch {
								*updatedBookmarkID = strings.TrimPrefix(req.URL.Path, "/api/bookmarks/")
								bodyBytes, _ := io.ReadAll(req.Body)
								if err := json.Unmarshal(bodyBytes, updatedBookmarkData); err != nil {
									t.Fatalf("Failed to unmarshal: %v", err)
								}
							}
							return &http.Response{
								StatusCode: http.StatusOK,
								Body:       io.NopCloser(strings.NewReader(`{"status": "ok"}`)),
								Header:     make(http.Header),
							}, nil
						},
					},
				}
			},
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
			mockHTTPClientFunc: func(t *testing.T, tc *koboSendTestCase, updatedBookmarkID *string, updatedBookmarkData *map[string]any, createdBookmarkURL *string) *http.Client {
				return &http.Client{
					Transport: &MockRoundTripper{
						RoundTripFunc: func(req *http.Request) (*http.Response, error) {
							if req.Method == http.MethodPost {
								var data struct {
									URL string `json:"url"`
								}
								bodyBytes, _ := io.ReadAll(req.Body)
								if err := json.Unmarshal(bodyBytes, &data); err != nil {
									t.Fatalf("Failed to unmarshal: %v", err)
								}
								*createdBookmarkURL = data.URL
							}
							return &http.Response{
								StatusCode: http.StatusOK,
								Body:       io.NopCloser(strings.NewReader(`{"status": "ok"}`)),
								Header:     make(http.Header),
							}, nil
						},
					},
				}
			},
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
			mockHTTPClientFunc: func(t *testing.T, tc *koboSendTestCase, updatedBookmarkID *string, updatedBookmarkData *map[string]any, createdBookmarkURL *string) *http.Client {
				return &http.Client{
					Transport: &MockRoundTripper{
						RoundTripFunc: func(req *http.Request) (*http.Response, error) {
							return &http.Response{
								StatusCode: http.StatusOK,
								Body:       io.NopCloser(strings.NewReader(`{"status": "ok"}`)),
								Header:     make(http.Header),
							}, nil
						},
					},
				}
			},
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
			mockHTTPClientFunc: func(t *testing.T, tc *koboSendTestCase, updatedBookmarkID *string, updatedBookmarkData *map[string]any, createdBookmarkURL *string) *http.Client {
				return &http.Client{
					Transport: &MockRoundTripper{
						RoundTripFunc: func(req *http.Request) (*http.Response, error) {
							return &http.Response{
								StatusCode: http.StatusOK,
								Body:       io.NopCloser(strings.NewReader(`{"status": "ok"}`)),
								Header:     make(http.Header),
							}, nil
						},
					},
				}
			},
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
			mockHTTPClientFunc: func(t *testing.T, tc *koboSendTestCase, updatedBookmarkID *string, updatedBookmarkData *map[string]any, createdBookmarkURL *string) *http.Client {
				return &http.Client{
					Transport: &MockRoundTripper{
						RoundTripFunc: func(req *http.Request) (*http.Response, error) {
							return &http.Response{
								StatusCode: http.StatusOK,
								Body:       io.NopCloser(strings.NewReader(`{"status": "ok"}`)),
								Header:     make(http.Header),
							}, nil
						},
					},
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset mock data
			updatedBookmarkID = ""
			updatedBookmarkData = nil
			createdBookmarkURL = ""

			app := NewApp(
				WithConfig(&config.Config{
					Users: []config.User{
						{
							Token:              mockDeviceToken,
							ReadeckAccessToken: mockPlaintextReadeckToken,
						},
					},
					Readeck: config.ConfigReadeck{Host: "http://mock-readeck.com"},
				}),
				WithLogger(testLogger),
				WithReadeckHTTPClient(tc.mockHTTPClientFunc(t, &tc, &updatedBookmarkID, &updatedBookmarkData, &createdBookmarkURL)),
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

// koboGetWithArchivedTestCase defines the structure for test cases in TestHandleKoboGetWithArchived.
type koboGetWithArchivedTestCase struct {
	name                string
	reqBody             *models.KoboGetRequest
	mockBookmarksSync   []readeck.BookmarkSync
	mockBookmarkDetails map[string]*readeck.Bookmark
	expectedStatus      int
	expectedListSize    int
	expectedTotal       int
	mockHTTPClientFunc  func(t *testing.T, tc *koboGetWithArchivedTestCase) *http.Client
}

func TestHandleKoboGetWithArchived(t *testing.T) {
	testCases := []koboGetWithArchivedTestCase{
		{
			name:    "successful get with archived",
			reqBody: &models.KoboGetRequest{Count: "10", AccessToken: mockDeviceToken},
			mockBookmarksSync: []readeck.BookmarkSync{
				{ID: "1", Type: "update"},
				{ID: "2", Type: "update"},
			},
			mockBookmarkDetails: map[string]*readeck.Bookmark{
				"1": {
					ID:         "1",
					Title:      "Test Bookmark",
					URL:        "http://example.com/bookmark1",
					Href:       "http://example.com/bookmark1",
					WordCount:  100,
					IsArchived: false,
				},
				"2": {
					ID:         "2",
					Title:      "Archived Bookmark",
					URL:        "http://example.com/bookmark2",
					Href:       "http://example.com/bookmark2",
					WordCount:  100,
					IsArchived: true,
				},
			},
			expectedStatus:   http.StatusOK,
			expectedListSize: 1,
			expectedTotal:    1,
			mockHTTPClientFunc: func(t *testing.T, tc *koboGetWithArchivedTestCase) *http.Client {
				return &http.Client{
					Transport: &MockRoundTripper{
						RoundTripFunc: func(req *http.Request) (*http.Response, error) {
							switch req.URL.Path {
							case "/api/bookmarks/sync":
								switch req.Method {
								case http.MethodGet:
									jsonBytes, err := json.Marshal(tc.mockBookmarksSync)
									if err != nil {
										return nil, err
									}
									return &http.Response{
										StatusCode: http.StatusOK,
										Body:       io.NopCloser(bytes.NewReader(jsonBytes)),
										Header:     make(http.Header),
									}, nil
								case http.MethodPost:
									boundary := "MULTIPART_BOUNDARY"
									var b bytes.Buffer
									writer := multipart.NewWriter(&b)
									if err := writer.SetBoundary(boundary); err != nil {
										return nil, err
									}

									reqBodyBytes, err := io.ReadAll(req.Body)
									if err != nil {
										return nil, err
									}
									var syncRequest struct {
										IDs []string `json:"id"`
									}
									if err := json.Unmarshal(reqBodyBytes, &syncRequest); err != nil {
										return nil, err
									}

									for _, id := range syncRequest.IDs {
										if bm, ok := tc.mockBookmarkDetails[id]; ok && bm != nil {
											partHeader := make(textproto.MIMEHeader)
											partHeader.Set("Content-Type", "application/json")
											partHeader.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="bookmark_%s.json"`, id))
											part, err := writer.CreatePart(partHeader)
											if err != nil {
												return nil, err
											}
											if err := json.NewEncoder(part).Encode(bm); err != nil {
												return nil, err
											}
										}
									}
									if err := writer.Close(); err != nil {
										return nil, err
									}

									header := make(http.Header)
									header.Set("Content-Type", fmt.Sprintf("multipart/mixed; boundary=%s", boundary))
									return &http.Response{
										StatusCode: http.StatusOK,
										Body:       io.NopCloser(bytes.NewReader(b.Bytes())),
										Header:     header,
									}, nil
								}
							}
							return &http.Response{
								StatusCode: http.StatusOK,
								Body:       io.NopCloser(strings.NewReader(`{"status": "ok"}`)),
								Header:     make(http.Header),
							}, nil
						},
					},
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			app := NewApp(
				WithConfig(&config.Config{
					Users: []config.User{
						{
							Token:              mockDeviceToken,
							ReadeckAccessToken: mockPlaintextReadeckToken,
						},
					},
					Readeck: config.ConfigReadeck{Host: "http://mock-readeck.com"},
				}),
				WithLogger(testLogger),
				WithReadeckHTTPClient(tc.mockHTTPClientFunc(t, &tc)),
			)

			reqBody := models.KoboGetRequest{Count: "10", AccessToken: mockDeviceToken} // Request more than the number of bookmarks
			body, err := json.Marshal(reqBody)
			if err != nil {
				t.Fatalf("Failed to marshal request body: %v", err)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/kobo/get", bytes.NewReader(body))
			rr := httptest.NewRecorder()

			app.HandleKoboGet(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
			}

			var resp models.KoboGetResponse
			if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}
			if len(resp.List) != 1 {
				t.Errorf("expected 1 item in list, got %d", len(resp.List))
			}
			if resp.Total != 1 {
				t.Errorf("expected total to be %d, got %d", tc.expectedTotal, resp.Total)
			}
			if _, ok := resp.List["2"]; ok {
				t.Error("archived bookmark should not be in the list")
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

