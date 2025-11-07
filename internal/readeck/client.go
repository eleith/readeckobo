package readeck

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	defaultHTTPTimeout = 10 * time.Second
)

// Client represents a Readeck API client.
type Client struct {
	BaseURL    *url.URL
	AccessToken string
	HTTPClient *http.Client
}

// NewClient creates a new Readeck API client.
// APIError represents an error returned by the Readeck API.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API error: %s (status: %d)", e.Message, e.StatusCode)
}

func NewClient(baseURL string, accessToken string) (*Client, error) {
	parsedURL, err := url.ParseRequestURI(baseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse base URL: %w", err)
	}

	return &Client{
		BaseURL:    parsedURL,
		AccessToken: accessToken,
		HTTPClient: &http.Client{
			Timeout: defaultHTTPTimeout,
		},
	}, nil
}

// doRequest performs an HTTP request and decodes the response.
func (c *Client) doRequest(ctx context.Context, method, path string, queryParams url.Values, body any, v any) (string, error) {
	reqURL := c.BaseURL.JoinPath(path)
	reqURL.RawQuery = queryParams.Encode()

	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return "", fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL.String(), reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	    if body != nil {
	        req.Header.Set("Content-Type", "application/json")
	    }
	
	    resp, err := c.HTTPClient.Do(req)
	    if err != nil {
	        return "", fmt.Errorf("failed to execute request: %w", err)
	    }
	    defer func() { _ = resp.Body.Close() }()
	
	    if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
	        return "", &APIError{StatusCode: resp.StatusCode, Message: resp.Status}
	    }
	if v != nil {
		if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
			return "", fmt.Errorf("failed to decode response body: %w", err)
		}
	}

	totalPages := resp.Header.Get("Total-Pages")
	return totalPages, nil
}

// GetBookmarksSync fetches bookmark synchronization events.
func (c *Client) GetBookmarksSync(ctx context.Context, since *time.Time) ([]BookmarkSync, error) {
	queryParams := url.Values{}
	if since != nil {
		queryParams.Add("since", strconv.FormatInt(since.Unix(), 10))
	}

	var bookmarks []BookmarkSync
	_, err := c.doRequest(ctx, http.MethodGet, "/api/bookmarks/sync", queryParams, nil, &bookmarks)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch bookmark syncs: %w", err)
	}

	return bookmarks, nil
}

// GetBookmarks fetches bookmarks for a specific site.
// This implementation does not handle pagination yet, it only fetches the first page.
// Pagination will be added later if needed.
func (c *Client) GetBookmarks(ctx context.Context, site string, page int, isArchived *bool) ([]Bookmark, int, error) {
	queryParams := url.Values{}
	if site != "" {
		queryParams.Add("site", site)
	}
	if page > 0 {
		queryParams.Add("page", strconv.Itoa(page))
	}
	if isArchived != nil {
		queryParams.Add("is_archived", strconv.FormatBool(*isArchived))
	}

	var bookmarks []Bookmark
	totalPagesStr, err := c.doRequest(ctx, http.MethodGet, "/api/bookmarks", queryParams, nil, &bookmarks)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch bookmarks: %w", err)
	}

	totalPages, err := strconv.Atoi(totalPagesStr)
	if err != nil {
		totalPages = 1 // Default to 1 if header is missing or invalid
	}

	return bookmarks, totalPages, nil
}

// GetBookmarkDetails fetches details for a single bookmark.
func (c *Client) GetBookmarkDetails(ctx context.Context, id string) (*Bookmark, error) {
	var bookmark Bookmark
	_, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/bookmarks/%s", id), nil, nil, &bookmark)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch bookmark details: %w", err)
	}

	return &bookmark, nil
}

// GetBookmarkArticle fetches the article content for a bookmark.
func (c *Client) GetBookmarkArticle(ctx context.Context, id string) (string, error) {
	reqURL := c.BaseURL.JoinPath(fmt.Sprintf("/api/bookmarks/%s/article", id))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, resp.Status)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	return string(bodyBytes), nil
}

// UpdateBookmark updates a bookmark.
func (c *Client) UpdateBookmark(ctx context.Context, id string, updates map[string]interface{}) error {
	path := fmt.Sprintf("/api/bookmarks/%s", id)
		_, err := c.doRequest(ctx, http.MethodPatch, path, nil, updates, nil)
	if err != nil {
		if apiErr, ok := err.(*APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			return nil
		}
		return fmt.Errorf("failed to update bookmark %s: %w", id, err)
	}
	return nil
}

// CreateBookmark creates a new bookmark.
func (c *Client) CreateBookmark(ctx context.Context, bookmarkURL string) error {
	body := map[string]string{"url": bookmarkURL}
	_, err := c.doRequest(ctx, http.MethodPost, "/api/bookmarks", nil, body, nil)
	if err != nil {
		return fmt.Errorf("failed to create bookmark: %w", err)
	}
	return nil
}
