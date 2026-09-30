package app

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	storeAPIHost          = "storeapi.kobo.com"
	maxInitializationBody = 8 << 20
)

var koboTransport = func() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 20 * time.Second
	return transport
}()

func (a *App) HandleStoreProxy(w http.ResponseWriter, r *http.Request) {
	a.serveKoboProxy(w, r, &url.URL{Scheme: "https", Host: storeAPIHost}, "")
}

// serveKoboProxy expects the public route prefix to have been removed from r.URL.
func (a *App) serveKoboProxy(w http.ResponseWriter, r *http.Request, target *url.URL, deviceToken string) {
	path := r.URL.Path
	if path == "" {
		path = "/"
	}
	proxy := a.newKoboProxy(target, path)
	if deviceToken != "" {
		originalDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			originalDirector(req)
			// Book services can build absolute URLs from forwarded headers.
			// Use the upstream origin, not the public ingress host.
			req.Header.Del("Forwarded")
			req.Header.Del("X-Forwarded-Port")
			req.Header.Del("X-Forwarded-Prefix")
			req.Header.Set("X-Forwarded-Host", target.Host)
			req.Header.Set("X-Forwarded-Proto", target.Scheme)
		}
	}
	if path == "/v1/initialization" {
		origin, err := a.publicOrigin(r)
		if err != nil {
			http.Error(w, "Invalid public origin", http.StatusBadGateway)
			return
		}
		configureInitializationProxy(proxy, origin+"/instapaper-proxy/instapaper")
	}
	proxy.ServeHTTP(w, r)
}

func (a *App) newKoboProxy(target *url.URL, path string) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		req.URL.Path = path
		originalDirector(req)
		req.Host = target.Host
	}
	if a.ProxyTransport != nil {
		proxy.Transport = a.ProxyTransport
	} else {
		proxy.Transport = koboTransport
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		a.Logger.Errorf("Kobo upstream proxy failed: %T", err)
		http.Error(w, "Bad gateway", http.StatusBadGateway)
	}
	return proxy
}

func configureInitializationProxy(proxy *httputil.ReverseProxy, instapaperURL string) {
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		// Request a complete document so its body can be rewritten.
		req.Header.Set("Accept-Encoding", "identity")
		req.Header.Del("If-None-Match")
		req.Header.Del("If-Modified-Since")
		req.Header.Del("Range")
		req.Header.Del("If-Range")
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		if resp.StatusCode == http.StatusPartialContent {
			return errors.New("partial initialization response")
		}
		if resp.StatusCode != http.StatusOK {
			return nil
		}
		return rewriteInitialization(resp, instapaperURL)
	}
}

func (a *App) publicOrigin(r *http.Request) (string, error) {
	if a.Config.PublicURL != "" {
		return strings.TrimRight(a.Config.PublicURL, "/"), nil
	}

	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "http"
		if r.TLS != nil {
			scheme = "https"
		}
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	if scheme != "http" && scheme != "https" || host == "" || strings.ContainsAny(host, "/@?#, \t\r\n\"\\") {
		return "", errors.New("invalid forwarded origin")
	}
	parsed, err := url.Parse("//" + host)
	if err != nil || parsed.Hostname() == "" || parsed.Host != host {
		return "", errors.New("invalid forwarded host")
	}
	return scheme + "://" + host, nil
}

func rewriteInitialization(resp *http.Response, instapaperURL string) error {
	originalBody := resp.Body
	defer func() { _ = originalBody.Close() }()

	var reader io.Reader = originalBody
	compressed := resp.Header.Get("Content-Encoding") == "gzip"
	if compressed {
		gz, err := gzip.NewReader(originalBody)
		if err != nil {
			return err
		}
		defer func() { _ = gz.Close() }()
		reader = gz
	} else if encoding := resp.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
		return fmt.Errorf("unsupported initialization encoding: %s", encoding)
	}

	body, err := io.ReadAll(io.LimitReader(reader, maxInitializationBody+1))
	if err != nil {
		return err
	}
	if len(body) > maxInitializationBody {
		return errors.New("initialization response too large")
	}
	encodedURL, err := json.Marshal(instapaperURL)
	if err != nil {
		return err
	}
	body = bytes.ReplaceAll(body, []byte("https://www.instapaper.com"), encodedURL[1:len(encodedURL)-1])
	if compressed {
		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		if _, err := gw.Write(body); err != nil {
			return err
		}
		if err := gw.Close(); err != nil {
			return err
		}
		body = buf.Bytes()
	} else {
		resp.Header.Del("Content-Encoding")
	}

	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
	for _, key := range []string{"ETag", "Last-Modified", "Content-MD5", "Digest", "Content-Range", "Accept-Ranges"} {
		resp.Header.Del(key)
	}
	resp.Header.Set("Cache-Control", "private, no-store")
	return nil
}
