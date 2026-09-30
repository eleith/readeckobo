package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func (a *App) HandleBookSync(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("deviceToken")
	upstream, known := a.bookSyncUpstream(token)
	if !known {
		http.NotFound(w, r)
		return
	}

	target, err := url.Parse(upstream)
	if err != nil {
		a.Logger.Errorf("Invalid configured book sync upstream: %T", err)
		http.Error(w, "Bad gateway", http.StatusBadGateway)
		return
	}

	http.StripPrefix("/booksync/"+token, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.serveKoboProxy(w, r, target, token)
	})).ServeHTTP(w, r)
}

func (a *App) bookSyncUpstream(token string) (string, bool) {
	for _, user := range a.Config.Users {
		if user.Token == token {
			if user.BookSyncURL != "" {
				return user.BookSyncURL, true
			}
			return "https://" + storeAPIHost, true
		}
	}
	return "", false
}

// rewriteBookImages keeps book-service cover URLs on the selected device's
// public route. Other initialization resources (including Kobo Store URLs) stay intact.
func rewriteBookImages(body []byte, upstream *url.URL, publicOrigin, publicRoute string) ([]byte, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(body, &document); err != nil {
		return nil, fmt.Errorf("decode book initialization: %w", err)
	}
	resourcesJSON, ok := document["Resources"]
	if !ok {
		return body, nil
	}
	var resources map[string]json.RawMessage
	if err := json.Unmarshal(resourcesJSON, &resources); err != nil {
		return nil, fmt.Errorf("decode book initialization resources: %w", err)
	}

	changed := false
	for _, field := range []string{"image_url_template", "image_url_quality_template"} {
		value, ok := resources[field]
		if !ok {
			continue
		}
		var imageURL string
		if err := json.Unmarshal(value, &imageURL); err != nil {
			return nil, fmt.Errorf("decode book %s: %w", field, err)
		}
		rewrittenURL, matches, err := rewriteBookURL(imageURL, upstream, publicRoute)
		if err != nil {
			return nil, fmt.Errorf("rewrite book %s: %w", field, err)
		}
		if !matches {
			continue
		}
		replacement, err := json.Marshal(rewrittenURL)
		if err != nil {
			return nil, err
		}
		resources[field] = replacement
		changed = true
	}
	if !changed {
		return body, nil
	}
	if imageHost, ok := resources["image_host"]; ok {
		var host string
		if err := json.Unmarshal(imageHost, &host); err != nil {
			return nil, fmt.Errorf("decode book image_host: %w", err)
		}
		hostURL, err := url.Parse(host)
		if err != nil {
			return nil, fmt.Errorf("parse book image_host: %w", err)
		}
		if sameBookOrigin(hostURL, upstream) {
			replacement, err := json.Marshal(publicOrigin)
			if err != nil {
				return nil, err
			}
			resources["image_host"] = replacement
		} else if strings.EqualFold(hostURL.Hostname(), upstream.Hostname()) {
			return nil, fmt.Errorf("book image_host uses an unexpected origin")
		}
	}
	updatedResources, err := json.Marshal(resources)
	if err != nil {
		return nil, err
	}
	document["Resources"] = updatedResources
	return json.Marshal(document)
}

// rewriteBookURL maps a URL on the configured upstream to this device's public
// route, keeping the original suffix so image template braces and queries survive.
func rewriteBookURL(value string, upstream *url.URL, publicRoute string) (string, bool, error) {
	imageURL, err := url.Parse(value)
	if err != nil {
		return "", false, err
	}
	prefix := strings.TrimRight(upstream.EscapedPath(), "/") + "/"
	if !strings.HasPrefix(imageURL.EscapedPath(), prefix) {
		return "", false, nil
	}
	if !sameBookOrigin(imageURL, upstream) {
		return "", false, fmt.Errorf("book image URL uses an unexpected origin")
	}
	// url.URL.String would escape the braces in Kobo image templates.
	rawPath := value[len(imageURL.Scheme)+3+len(imageURL.Host):]
	if !strings.HasPrefix(rawPath, prefix) {
		return "", false, fmt.Errorf("book image URL uses an unexpected path encoding")
	}
	return publicRoute + strings.TrimPrefix(rawPath, strings.TrimSuffix(prefix, "/")), true, nil
}

func sameBookOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) &&
		strings.EqualFold(a.Hostname(), b.Hostname()) && bookPort(a) == bookPort(b)
}

func bookPort(u *url.URL) string {
	if u.Port() != "" {
		return u.Port()
	}
	if strings.EqualFold(u.Scheme, "https") {
		return "443"
	}
	return "80"
}
