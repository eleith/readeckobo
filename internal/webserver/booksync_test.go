package webserver

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"readeckobo/internal/app"
	"readeckobo/internal/config"
	"readeckobo/internal/logger"
)

func TestBookSyncRoutes(t *testing.T) {
	for _, tt := range []struct {
		name, path, scheme, host, upstreamPath, escapedPath, query string
		wantStatus                                                 int
	}{
		{"custom endpoint with path prefix", "/booksync/books/v1/library/sync?page=2", "https", "books.example.com", "/api/kobo/device/v1/library/sync", "/api/kobo/device/v1/library/sync", "page=2", http.StatusAccepted},
		{"custom HTTP endpoint", "/booksync/httpbook/v1/library/sync", "http", "localhost:8081", "/kobo/v1/library/sync", "/kobo/v1/library/sync", "", http.StatusAccepted},
		{"escaped suffix", "/booksync/books/v1/%2Fbooks?a=1%3B2", "https", "books.example.com", "/api/kobo/device/v1//books", "/api/kobo/device/v1/%2Fbooks", "a=1%3B2", http.StatusAccepted},
		{"custom endpoint root", "/booksync/books/", "https", "books.example.com", "/api/kobo/device/", "/api/kobo/device/", "", http.StatusAccepted},
		{"known token uses Kobo Store", "/booksync/store/v1/library/sync?page=3", "https", "storeapi.kobo.com", "/v1/library/sync", "/v1/library/sync", "page=3", http.StatusAccepted},
		{"unknown token", "/booksync/unknown/v1/library/sync", "", "", "", "", "", http.StatusNotFound},
		{"partial token does not match", "/booksync/book/v1/library/sync", "", "", "", "", "", http.StatusNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls int
			application := app.NewApp(
				app.WithConfig(&config.Config{Users: []config.User{
					{Token: "books", BookSyncURL: "https://books.example.com/api/kobo/device"},
					{Token: "httpbook", BookSyncURL: "http://localhost:8081/kobo"},
					{Token: "store"},
				}}),
				app.WithLogger(logger.New(logger.ERROR)),
				app.WithProxyTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
					calls++
					if req.URL.Scheme != tt.scheme || req.URL.Host != tt.host || req.Host != tt.host || req.URL.Path != tt.upstreamPath || req.URL.EscapedPath() != tt.escapedPath || req.URL.RawQuery != tt.query {
						t.Errorf("upstream URL = %s, Host = %q; want %s://%s, path %q, escaped %q, query %q", req.URL, req.Host, tt.scheme, tt.host, tt.upstreamPath, tt.escapedPath, tt.query)
					}
					if req.Method != http.MethodPost || req.Header.Get("X-Test") != "forwarded" {
						t.Errorf("upstream method = %s, X-Test = %q", req.Method, req.Header.Get("X-Test"))
					}
					body, err := io.ReadAll(req.Body)
					if err != nil || string(body) != "payload" {
						t.Errorf("upstream body = %q, err = %v", body, err)
					}
					return &http.Response{StatusCode: http.StatusAccepted, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("book response"))}, nil
				})),
			)
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader("payload"))
			req.Header.Set("X-Test", "forwarded")
			NewHandler(application, logger.New(logger.ERROR)).ServeHTTP(rr, req)
			if rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rr.Code, tt.wantStatus)
			}
			if tt.host == "" {
				if calls != 0 {
					t.Errorf("unknown token reached upstream (%d calls)", calls)
				}
			} else if calls != 1 || rr.Body.String() != "book response" {
				t.Errorf("upstream calls = %d, body = %q; want one call and book response", calls, rr.Body.String())
			}
		})
	}
}

func TestBookSyncDoesNotRetryConfiguredService(t *testing.T) {
	var calls int
	application := app.NewApp(
		app.WithConfig(&config.Config{Users: []config.User{{Token: "books", BookSyncURL: "https://books.example.com/api/kobo/device"}}}),
		app.WithLogger(logger.New(logger.ERROR)),
		app.WithProxyTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			if req.URL.Host != "books.example.com" {
				t.Errorf("upstream host = %q, want books.example.com", req.URL.Host)
			}
			return nil, errors.New("book service unavailable")
		})),
	)
	rr := httptest.NewRecorder()
	NewHandler(application, logger.New(logger.ERROR)).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/booksync/books/v1/library/sync", nil))
	if rr.Code != http.StatusBadGateway || calls != 1 {
		t.Errorf("status = %d, upstream calls = %d; want 502 and one call", rr.Code, calls)
	}
}

func TestBookSyncInitialization(t *testing.T) {
	for _, tt := range []struct {
		name, token, upstreamHost, upstreamPath string
		gzip                                    bool
	}{
		{"custom plaintext", "books", "books.example.com", "/api/kobo/device/v1/initialization", false},
		{"custom gzip", "books", "books.example.com", "/api/kobo/device/v1/initialization", true},
		{"store fallback", "store", "storeapi.kobo.com", "/v1/initialization", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{"instapaper":"https://www.instapaper.com"}`)
			header := http.Header{"Content-Type": {"application/json"}, "Etag": {`"old"`}}
			if tt.gzip {
				var b bytes.Buffer
				writer := gzip.NewWriter(&b)
				if _, err := writer.Write(body); err != nil {
					t.Fatal(err)
				}
				if err := writer.Close(); err != nil {
					t.Fatal(err)
				}
				body = b.Bytes()
				header.Set("Content-Encoding", "gzip")
			}
			application := app.NewApp(
				app.WithConfig(&config.Config{PublicURL: "https://reader.example.com", Users: []config.User{
					{Token: "books", BookSyncURL: "https://books.example.com/api/kobo/device"}, {Token: "store"},
				}}),
				app.WithLogger(logger.New(logger.ERROR)),
				app.WithProxyTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
					if req.URL.Host != tt.upstreamHost || req.URL.Path != tt.upstreamPath || req.URL.RawQuery != "locale=en" || req.Header.Get("Accept-Encoding") != "identity" || req.Header.Get("Range") != "" || req.Header.Get("If-None-Match") != "" {
						t.Errorf("unexpected initialization upstream: URL=%s, headers=%v", req.URL, req.Header)
					}
					return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(bytes.NewReader(body))}, nil
				})),
			)
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/booksync/"+tt.token+"/v1/initialization?locale=en", nil)
			req.Header.Set("If-None-Match", `"old"`)
			req.Header.Set("Range", "bytes=0-10")
			NewHandler(application, logger.New(logger.ERROR)).ServeHTTP(rr, req)
			if rr.Code != http.StatusOK || rr.Header().Get("ETag") != "" {
				t.Fatalf("status = %d, headers = %v", rr.Code, rr.Header())
			}
			if got := rr.Header().Get("Content-Length"); got != strconv.Itoa(rr.Body.Len()) {
				t.Errorf("Content-Length = %q, body size = %d", got, rr.Body.Len())
			}
			var result []byte
			var err error
			if tt.gzip {
				if rr.Header().Get("Content-Encoding") != "gzip" {
					t.Errorf("Content-Encoding = %q, want gzip", rr.Header().Get("Content-Encoding"))
				}
				reader, gzipErr := gzip.NewReader(rr.Body)
				if gzipErr != nil {
					t.Fatal(gzipErr)
				}
				result, err = io.ReadAll(reader)
				_ = reader.Close()
			} else {
				result = rr.Body.Bytes()
			}
			if err != nil || string(result) != `{"instapaper":"https://reader.example.com/instapaper-proxy/instapaper"}` {
				t.Errorf("rewritten body = %q, err = %v", result, err)
			}
		})
	}
}

func TestBookSyncStreamsDownload(t *testing.T) {
	reader, writer := io.Pipe()
	application := app.NewApp(
		app.WithConfig(&config.Config{Users: []config.User{{Token: "books", BookSyncURL: "https://books.example.com/api/kobo/device"}}}),
		app.WithLogger(logger.New(logger.ERROR)),
		app.WithProxyTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/api/kobo/device/v1/books/1/download" {
				t.Errorf("upstream path = %q", req.URL.Path)
			}
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/epub+zip"}}, Body: reader, ContentLength: -1}, nil
		})),
	)
	server := httptest.NewServer(NewHandler(application, logger.New(logger.ERROR)))
	defer server.Close()
	defer func() { _ = writer.Close() }()

	firstChunk := bytes.Repeat([]byte("x"), 32*1024)
	result := make(chan error, 1)
	go func() {
		resp, err := server.Client().Get(server.URL + "/booksync/books/v1/books/1/download")
		if err != nil {
			result <- err
			return
		}
		defer func() { _ = resp.Body.Close() }()
		data := make([]byte, len(firstChunk))
		_, err = io.ReadFull(resp.Body, data)
		if err == nil && !bytes.Equal(data, firstChunk) {
			err = io.ErrUnexpectedEOF
		}
		result <- err
	}()
	go func() { _, _ = writer.Write(firstChunk) }()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("book download did not stream before upstream completion")
	}
}
