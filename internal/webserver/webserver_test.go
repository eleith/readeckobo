package webserver

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"readeckobo/internal/app"
	"readeckobo/internal/config"
	"readeckobo/internal/logger"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestStoreRoutes(t *testing.T) {
	application := app.NewApp(
		app.WithConfig(&config.Config{PublicURL: "https://reader.example.com"}),
		app.WithLogger(logger.New(logger.ERROR)),
		app.WithProxyTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host != "storeapi.kobo.com" || req.Host != "storeapi.kobo.com" {
				t.Errorf("upstream host = %q, Host = %q", req.URL.Host, req.Host)
			}
			switch req.URL.Path {
			case "/", "/v1/initialization", "/v1/library/sync", "/v1//books":
			default:
				t.Errorf("unexpected upstream path = %q", req.URL.Path)
			}
			if req.URL.Path == "/v1//books" && req.URL.EscapedPath() != "/v1/%2Fbooks" {
				t.Errorf("escaped upstream path = %q, want /v1/%%2Fbooks", req.URL.EscapedPath())
			}
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"url":"https://www.instapaper.com"}`))}, nil
		})),
	)
	handler := NewHandler(application, logger.New(logger.ERROR))
	for _, path := range []string{"/instapaper-proxy/storeapi/v1/initialization", "/instapaper-proxy/storeapi/v1/library/sync", "/instapaper-proxy/storeapi/v1/%2Fbooks?a=1%3B2", "/instapaper-proxy/storeapi"} {
		t.Run(path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
			if rr.Code != http.StatusOK {
				t.Errorf("status = %d, want 200", rr.Code)
			}
		})
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/instapaper-proxy/storeapi-nope", nil))
	if rr.Code != http.StatusNotFound {
		t.Errorf("unknown store route status = %d, want 404", rr.Code)
	}
}

func TestArticleRoutes(t *testing.T) {
	readeckServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/bookmarks/sync" {
			t.Errorf("unexpected Readeck request: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "[]")
	}))
	defer readeckServer.Close()

	application := app.NewApp(
		app.WithConfig(&config.Config{
			Readeck: config.ConfigReadeck{Host: readeckServer.URL},
			Users:   []config.User{{Token: "device", ReadeckAccessToken: "readeck"}},
		}),
		app.WithLogger(logger.New(logger.ERROR)),
		app.WithReadeckHTTPClient(readeckServer.Client()),
	)
	handler := NewHandler(application, logger.New(logger.ERROR))
	for _, tt := range []struct {
		name, path, body, wantBody string
		wantStatus                 int
	}{
		{"get", "/instapaper-proxy/instapaper/api/kobo/get", `{"access_token":"device"}`, `"status":1`, http.StatusOK},
		{"download", "/instapaper-proxy/instapaper/api/kobo/download", `{"access_token":"device"}`, "Missing 'url' parameter", http.StatusBadRequest},
		{"send", "/instapaper-proxy/instapaper/api/kobo/send", `{"access_token":"device","actions":[]}`, `"status":true`, http.StatusOK},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			handler.ServeHTTP(rr, req)
			if rr.Code != tt.wantStatus || !strings.Contains(rr.Body.String(), tt.wantBody) {
				t.Errorf("status = %d, body = %q; want %d and %q", rr.Code, rr.Body.String(), tt.wantStatus, tt.wantBody)
			}
		})
	}
}

func TestRequestLogsDoNotExposeTokens(t *testing.T) {
	var logs bytes.Buffer
	old := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(old)

	application := app.NewApp(
		app.WithConfig(&config.Config{}),
		app.WithLogger(logger.New(logger.DEBUG)),
		app.WithImageHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, io.ErrUnexpectedEOF
		})}),
	)
	handler := NewHandler(application, logger.New(logger.DEBUG))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/booksync/private-token/?access_token=query-secret", nil))
	req := httptest.NewRequest(http.MethodPost, "/instapaper-proxy/instapaper/api/kobo/get", strings.NewReader(`{"access_token":"body-secret"}`))
	handler.ServeHTTP(httptest.NewRecorder(), req)
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/convert-image?url=https%3A%2F%2Fexample.com%2Fimg%3Ftoken%3Dimage-secret", nil))
	for _, secret := range []string{"private-token", "query-secret", "body-secret", "image-secret"} {
		if strings.Contains(logs.String(), secret) {
			t.Errorf("request logs contain secret %q", secret)
		}
	}
}
