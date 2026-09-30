package app

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"readeckobo/internal/config"
)

func storeHandler(a *App) http.Handler {
	return http.StripPrefix("/instapaper-proxy/storeapi", http.HandlerFunc(a.HandleStoreProxy))
}

func TestHandleStoreProxy(t *testing.T) {
	t.Run("forwards requests to the fixed store origin", func(t *testing.T) {
		body := "book sync response"
		a := NewApp(WithConfig(&config.Config{}), WithLogger(testLogger), WithProxyTransport(&MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				if req.URL.Scheme != "https" || req.URL.Host != storeAPIHost || req.Host != storeAPIHost || req.URL.Path != "/v1/library/sync" || req.URL.RawQuery != "page=2" {
					t.Errorf("unexpected upstream request: URL=%s Host=%q", req.URL, req.Host)
				}
				data, err := io.ReadAll(req.Body)
				if err != nil || string(data) != "payload" {
					t.Errorf("upstream body = %q, err = %v", data, err)
				}
				if req.Header.Get("X-Test") != "forwarded" {
					t.Errorf("X-Test = %q", req.Header.Get("X-Test"))
				}
				return &http.Response{StatusCode: http.StatusAccepted, Header: http.Header{"Content-Type": {"application/octet-stream"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
			},
		}))
		req := httptest.NewRequest(http.MethodPost, "/instapaper-proxy/storeapi/v1/library/sync?page=2", strings.NewReader("payload"))
		req.Header.Set("X-Test", "forwarded")
		rr := httptest.NewRecorder()
		storeHandler(a).ServeHTTP(rr, req)
		if rr.Code != http.StatusAccepted || rr.Body.String() != body {
			t.Errorf("status = %d, body length = %d; want 202 and %d", rr.Code, rr.Body.Len(), len(body))
		}
	})

	t.Run("streams an ordinary response before upstream completion", func(t *testing.T) {
		reader, writer := io.Pipe()
		a := NewApp(WithConfig(&config.Config{}), WithLogger(testLogger), WithProxyTransport(&MockRoundTripper{
			RoundTripFunc: func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode:    http.StatusOK,
					Header:        http.Header{"Content-Type": {"application/octet-stream"}},
					Body:          reader,
					ContentLength: -1,
				}, nil
			},
		}))
		server := httptest.NewServer(storeHandler(a))
		defer server.Close()
		defer func() { _ = writer.Close() }()

		firstChunk := bytes.Repeat([]byte("x"), 32*1024)
		result := make(chan error, 1)
		go func() {
			resp, err := server.Client().Get(server.URL + "/instapaper-proxy/storeapi/v1/library/sync")
			if err != nil {
				result <- err
				return
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusOK {
				result <- errors.New("unexpected status")
				return
			}
			data := make([]byte, len(firstChunk))
			_, err = io.ReadFull(resp.Body, data)
			if err == nil && !bytes.Equal(data, firstChunk) {
				err = errors.New("unexpected bytes in first chunk")
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
			t.Fatal("proxy did not deliver bytes before upstream finished")
		}
	})

	for _, tt := range []struct {
		name       string
		gzip       bool
		publicURL  string
		wantOrigin string
	}{
		{name: "plaintext uses configured public origin", publicURL: "https://reader.example.com/", wantOrigin: "https://reader.example.com"},
		{name: "gzipped response uses trusted forwarded origin", gzip: true, wantOrigin: "https://reader.example.com"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			upstreamBody := []byte(`{"instapaper":"https://www.instapaper.com"}`)
			responseHeader := http.Header{
				"Content-Type":  {"application/json"},
				"Etag":          {`"old-tag"`},
				"Last-Modified": {"Tue, 01 Jan 2019 00:00:00 GMT"},
			}
			if tt.gzip {
				var buf bytes.Buffer
				writer := gzip.NewWriter(&buf)
				if _, err := writer.Write(upstreamBody); err != nil {
					t.Fatal(err)
				}
				if err := writer.Close(); err != nil {
					t.Fatal(err)
				}
				upstreamBody = buf.Bytes()
				responseHeader.Set("Content-Encoding", "gzip")
			}
			responseHeader.Set("Content-Length", strconv.Itoa(len(upstreamBody)))
			a := NewApp(WithConfig(&config.Config{PublicURL: tt.publicURL}), WithLogger(testLogger), WithProxyTransport(&MockRoundTripper{
				RoundTripFunc: func(req *http.Request) (*http.Response, error) {
					if req.URL.Path != "/v1/initialization" || req.URL.RawQuery != "locale=en" || req.Header.Get("Accept-Encoding") != "identity" || req.Header.Get("If-None-Match") != "" || req.Header.Get("Range") != "" || req.Header.Get("If-Range") != "" {
						t.Errorf("unexpected initialization request: %s, headers: %v", req.URL, req.Header)
					}
					return &http.Response{StatusCode: http.StatusOK, Header: responseHeader, Body: io.NopCloser(bytes.NewReader(upstreamBody))}, nil
				},
			}))
			req := httptest.NewRequest(http.MethodGet, "/instapaper-proxy/storeapi/v1/initialization?locale=en", nil)
			req.Host = "untrusted.example.com"
			forwardedHost := "reader.example.com"
			if tt.publicURL != "" {
				forwardedHost = "ignored.example.com"
			}
			req.Header.Set("X-Forwarded-Host", forwardedHost)
			req.Header.Set("X-Forwarded-Proto", "https")
			req.Header.Set("If-None-Match", `"old-tag"`)
			req.Header.Set("Range", "bytes=0-50")
			req.Header.Set("If-Range", `"old-tag"`)
			rr := httptest.NewRecorder()
			storeHandler(a).ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
			}
			if got := rr.Header().Get("Content-Length"); got != strconv.Itoa(rr.Body.Len()) {
				t.Errorf("Content-Length = %q, body size = %d", got, rr.Body.Len())
			}
			if rr.Header().Get("ETag") != "" || rr.Header().Get("Last-Modified") != "" || rr.Header().Get("Cache-Control") != "private, no-store" {
				t.Errorf("stale response headers: %v", rr.Header())
			}
			var result []byte
			if tt.gzip {
				if rr.Header().Get("Content-Encoding") != "gzip" {
					t.Errorf("Content-Encoding = %q", rr.Header().Get("Content-Encoding"))
				}
				reader, err := gzip.NewReader(rr.Body)
				if err != nil {
					t.Fatal(err)
				}
				result, err = io.ReadAll(reader)
				if err != nil {
					t.Fatal(err)
				}
				_ = reader.Close()
			} else {
				if rr.Header().Get("Content-Encoding") != "" {
					t.Errorf("Content-Encoding = %q", rr.Header().Get("Content-Encoding"))
				}
				result = rr.Body.Bytes()
			}
			want := `{"instapaper":"` + tt.wantOrigin + `/instapaper-proxy/instapaper"}`
			if string(result) != want {
				t.Errorf("body = %q, want %q", result, want)
			}
		})
	}

	t.Run("preserves escaped path and query", func(t *testing.T) {
		a := NewApp(WithConfig(&config.Config{}), WithLogger(testLogger), WithProxyTransport(&MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/v1//books" || req.URL.EscapedPath() != "/v1/%2Fbooks" || req.URL.RawQuery != "a=1%3B2" {
					t.Errorf("upstream URL = %q, RawPath = %q", req.URL, req.URL.RawPath)
				}
				return &http.Response{StatusCode: http.StatusNoContent, Header: http.Header{}, Body: http.NoBody}, nil
			},
		}))
		rr := httptest.NewRecorder()
		storeHandler(a).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/instapaper-proxy/storeapi/v1/%2Fbooks?a=1%3B2", nil))
		if rr.Code != http.StatusNoContent {
			t.Errorf("status = %d, want 204", rr.Code)
		}
	})

	for _, tt := range []struct {
		name, encoding, body string
	}{
		{name: "rejects invalid gzip initialization", encoding: "gzip", body: "not gzip"},
		{name: "rejects unsupported initialization encoding", encoding: "br", body: "compressed"},
		{name: "limits initialization size", body: strings.Repeat("x", maxInitializationBody+1)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a := NewApp(WithConfig(&config.Config{PublicURL: "https://reader.example.com"}), WithLogger(testLogger), WithProxyTransport(&MockRoundTripper{
				RoundTripFunc: func(*http.Request) (*http.Response, error) {
					header := http.Header{}
					header.Set("Content-Encoding", tt.encoding)
					return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader(tt.body))}, nil
				},
			}))
			rr := httptest.NewRecorder()
			storeHandler(a).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/instapaper-proxy/storeapi/v1/initialization", nil))
			if rr.Code != http.StatusBadGateway {
				t.Errorf("status = %d, want 502", rr.Code)
			}
		})
	}

	t.Run("partial initialization is rejected", func(t *testing.T) {
		a := NewApp(WithConfig(&config.Config{PublicURL: "https://reader.example.com"}), WithLogger(testLogger), WithProxyTransport(&MockRoundTripper{
			RoundTripFunc: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusPartialContent, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("partial"))}, nil
			},
		}))
		rr := httptest.NewRecorder()
		storeHandler(a).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/instapaper-proxy/storeapi/v1/initialization", nil))
		if rr.Code != http.StatusBadGateway {
			t.Errorf("status = %d, want 502", rr.Code)
		}
	})

	t.Run("non-OK initialization passes through without rewriting", func(t *testing.T) {
		body := `{"url":"https://www.instapaper.com"}`
		a := NewApp(WithConfig(&config.Config{PublicURL: "https://reader.example.com"}), WithLogger(testLogger), WithProxyTransport(&MockRoundTripper{
			RoundTripFunc: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusNotFound, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
			},
		}))
		rr := httptest.NewRecorder()
		storeHandler(a).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/instapaper-proxy/storeapi/v1/initialization", nil))
		if rr.Code != http.StatusNotFound || rr.Body.String() != body {
			t.Errorf("status = %d, body = %q; want 404 and unchanged body", rr.Code, rr.Body.String())
		}
	})

	t.Run("encodes rewritten URLs as valid JSON strings", func(t *testing.T) {
		urlWithQuote := `https://reader.example.com/path?value="quoted"`
		resp := &http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"instapaper":"https://www.instapaper.com"}`))}
		if err := rewriteInitialization(resp, urlWithQuote, nil); err != nil {
			t.Fatal(err)
		}
		var decoded struct {
			Instapaper string `json:"instapaper"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
			t.Fatalf("rewritten response is invalid JSON: %v", err)
		}
		if decoded.Instapaper != urlWithQuote {
			t.Errorf("instapaper URL = %q, want %q", decoded.Instapaper, urlWithQuote)
		}
	})

	t.Run("rejects malformed forwarded origin", func(t *testing.T) {
		a := NewApp(WithConfig(&config.Config{}), WithLogger(testLogger))
		req := httptest.NewRequest(http.MethodGet, "/instapaper-proxy/storeapi/v1/initialization", nil)
		req.Header.Set("X-Forwarded-Proto", "javascript")
		rr := httptest.NewRecorder()
		storeHandler(a).ServeHTTP(rr, req)
		if rr.Code != http.StatusBadGateway {
			t.Errorf("status = %d, want 502", rr.Code)
		}
	})

	t.Run("rejects quoted forwarded host", func(t *testing.T) {
		a := NewApp(WithConfig(&config.Config{}), WithLogger(testLogger))
		req := httptest.NewRequest(http.MethodGet, "/instapaper-proxy/storeapi/v1/initialization", nil)
		req.Header.Set("X-Forwarded-Host", `reader.example.com"injected`)
		rr := httptest.NewRecorder()
		storeHandler(a).ServeHTTP(rr, req)
		if rr.Code != http.StatusBadGateway {
			t.Errorf("status = %d, want 502", rr.Code)
		}
	})

	t.Run("returns 502 on upstream failure", func(t *testing.T) {
		a := NewApp(WithConfig(&config.Config{}), WithLogger(testLogger), WithProxyTransport(&MockRoundTripper{
			RoundTripFunc: func(*http.Request) (*http.Response, error) { return nil, errors.New("upstream unavailable") },
		}))
		rr := httptest.NewRecorder()
		storeHandler(a).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/instapaper-proxy/storeapi/v1/library/sync", nil))
		if rr.Code != http.StatusBadGateway {
			t.Errorf("status = %d, want 502", rr.Code)
		}
	})

	t.Run("does not rewrite other initialization-like paths", func(t *testing.T) {
		body := `{"url":"https://www.instapaper.com"}`
		a := NewApp(WithConfig(&config.Config{}), WithLogger(testLogger), WithProxyTransport(&MockRoundTripper{
			RoundTripFunc: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
			},
		}))
		rr := httptest.NewRecorder()
		storeHandler(a).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/instapaper-proxy/storeapi/v2/initialization", nil))
		if rr.Body.String() != body {
			t.Errorf("body = %q, want %q", rr.Body.String(), body)
		}
	})
}
