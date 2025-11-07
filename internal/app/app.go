package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
	"golang.org/x/net/html"
	"readeckobo/internal/config"
	"readeckobo/internal/readeck"
)

// App holds the application's core dependencies and configuration.
type App struct {
	Config        *config.Config
	ReadeckClient readeck.ClientInterface // Use the interface
}

// Option is a functional option for configuring the App.
type Option func(*App)

// NewApp creates a new App instance with the given options.
func NewApp(opts ...Option) *App {
	app := &App{}
	for _, opt := range opts {
		opt(app)
	}
	return app
}

// WithConfig sets the application configuration.
func WithConfig(cfg *config.Config) Option {
	return func(a *App) {
		a.Config = cfg
	}
}

// WithReadeckClient sets the Readeck API client.
func WithReadeckClient(client readeck.ClientInterface) Option { // Accept the interface
	return func(a *App) {
		a.ReadeckClient = client
	}
}

// GetRequest represents the incoming request for /api/kobo/get
type GetRequest struct {
	AccessToken string     `json:"access_token"`
	ConsumerKey string     `json:"consumer_key"`
	ContentType string     `json:"contentType"`
	Count       string     `json:"count"`
	DetailType  string     `json:"detailType"`
	Offset      string     `json:"offset"`
	State       string     `json:"state"`
	Total       string     `json:"total"`
	Since       *time.Time `json:"since"`
}

// KoboGetResponse represents the outgoing response for /api/kobo/get
type KoboGetResponse struct {
	Status int            `json:"status"`
	List   map[string]any `json:"list"`
	Total  int            `json:"total"`
}

// HandleKoboGet handles the /api/kobo/get endpoint.
func (a *App) HandleKoboGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		log.Printf("Error reading /api/kobo/get request body: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
		return
	}
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes)) // Restore the body for subsequent reads

	var req GetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		log.Printf("Error decoding /api/kobo/get request: %v, body: %s, URL: %s, Params: %v", err, string(bodyBytes), r.URL.Path, r.URL.Query())
		return
	}

	count, _ := strconv.Atoi(req.Count)
	offset, _ := strconv.Atoi(req.Offset)

	ctx := r.Context()
	bsyncs, err := a.ReadeckClient.GetBookmarksSync(ctx, req.Since)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get bookmark syncs: %v", err), http.StatusInternalServerError)
		log.Printf("Error getting bookmark syncs for /api/kobo/get: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
		return
	}

	resultList := make(map[string]any)
	processedCount := 0
	totalBookmarks := len(bsyncs)

	for i, bsync := range bsyncs {
		if i < offset {
			continue
		}
		if processedCount >= count && count != 0 {
			break
		}

		if bsync.Type == "delete" {
			resultList[bsync.ID] = map[string]any{
				"item_id": bsync.ID,
				"status":  "2",
			}
		} else {
			bookmark, err := a.ReadeckClient.GetBookmarkDetails(ctx, bsync.ID)
			if err != nil {
				log.Printf("Error getting bookmark details for ID %s in /api/kobo/get: %v, URL: %s, Params: %v", bsync.ID, err, r.URL.Path, r.URL.Query())
				continue
			}

			if bookmark == nil {
				log.Printf("Bookmark details for ID %s not found in /api/kobo/get, URL: %s, Params: %v", bsync.ID, r.URL.Path, r.URL.Query())
				continue
			}

			if bookmark.IsArchived {
				totalBookmarks--
				continue
			}

			hasImage := "0"
			image := map[string]any{"src": ""}
			images := map[string]any{}
			optional := map[string]any{}

			if bookmark.Resources.Image != nil && bookmark.Resources.Image.Src != "" {
				hasImage = "1"
				image["src"] = bookmark.Resources.Image.Src
				images["1"] = map[string]any{
					"image_id": "1",
					"item_id":  "1",
					"src":      bookmark.Resources.Image.Src,
				}
				optional["top_image_url"] = bookmark.Resources.Image.Src
			}

			tags := make(map[string]any)
			for _, label := range bookmark.Labels {
				tags[label] = map[string]string{"item_id": bsync.ID, "tag": label}
			}

			resultList[bsync.ID] = map[string]any{
				"authors":        map[string]any{},
				"excerpt":        bookmark.Description,
				"favorite":       "0",
				"given_title":    bookmark.Title,
				"given_url":      bookmark.URL,
				"has_image":      hasImage,
				"has_video":      "0",
				"image":          image,
				"images":         images,
				"is_article":     "1",
				"item_id":        bookmark.ID,
				"resolved_id":    bookmark.ID,
				"resolved_title": bookmark.Title,
				"resolved_url":   bookmark.URL,
				"status":         "0",
				"tags":           tags,
				"time_added":     bookmark.Created.Unix(),
				"time_read":      0,
				"time_updated":   bookmark.Updated.Unix(),
				"videos":         []any{},
				"word_count":     bookmark.WordCount,
				"_optional":      optional,
			}
			for _, author := range bookmark.Authors {
				resultList[bsync.ID].(map[string]any)["authors"].(map[string]any)[author] = map[string]string{"author_id": author, "name": author}
			}
		}
		processedCount++
	}

	resp := KoboGetResponse{
		Status: 1,
		List:   resultList,
		Total:  totalBookmarks,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				log.Printf("Error encoding response for /api/kobo/get: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
			}}

// DownloadRequest represents the incoming request for /api/kobo/download
type DownloadRequest struct {
	AccessToken string `json:"access_token"`
	ConsumerKey string `json:"consumer_key"`
	Images      int    `json:"images"`
	Refresh     int    `json:"refresh"`
	Output      string `json:"output"`
	URL         string `json:"url"`
}

// HandleKoboDownload handles the /api/kobo/download endpoint.
func (a *App) HandleKoboDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		log.Printf("Error parsing form for /api/kobo/download: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
		return
	}

	reqURLStr := r.FormValue("url")
	if reqURLStr == "" {
		http.Error(w, "Missing 'url' parameter", http.StatusBadRequest)
		log.Printf("Error: Missing 'url' parameter in /api/kobo/download request, URL: %s, Params: %v", r.URL.Path, r.URL.Query())
		return
	}

	parsedURL, err := url.Parse(reqURLStr)
	if err != nil {
		http.Error(w, "Invalid 'url' parameter", http.StatusBadRequest)
		log.Printf("Error: Invalid 'url' parameter in /api/kobo/download request: %v, url: %s, URL: %s, Params: %v", err, reqURLStr, r.URL.Path, r.URL.Query())
		return
	}

	var bookmarkFound *readeck.Bookmark
	sitesToTry := getSitesToTry(parsedURL.Host)
	ctx := r.Context()

	for _, site := range sitesToTry {
		currentPage := 1
		totalPages := 1 // Initialize to 1 to ensure at least one page is fetched

		for currentPage <= totalPages {
			isArchived := false
			bookmarks, tp, err := a.ReadeckClient.GetBookmarks(ctx, site, currentPage, &isArchived)
			if err != nil {
				log.Printf("Error searching Readeck bookmarks for site %s, page %d in /api/kobo/download: %v, URL: %s, Params: %v", site, currentPage, err, r.URL.Path, r.URL.Query())
				break // Break from inner loop, try next site
			}
			totalPages = tp // Update totalPages from the response header

			for i := range bookmarks {
				if bookmarks[i].URL != "" && bookmarks[i].URL == reqURLStr {
					bookmarkFound = &bookmarks[i]
					break // Found the bookmark, break from inner loop
				}
			}
			if bookmarkFound != nil {
				break // Found the bookmark, break from outer loop
			}
			currentPage++
		}
		if bookmarkFound != nil {
			break // Found the bookmark, break from outermost loop
		}
	}

	if bookmarkFound == nil {
		http.Error(w, "Article not found", http.StatusNotFound)
		return
	}

	articleHTML, err := a.ReadeckClient.GetBookmarkArticle(ctx, bookmarkFound.ID)
	if err != nil {
		http.Error(w, "Failed to fetch article content", http.StatusInternalServerError)
		log.Printf("Error fetching article content for bookmark %s in /api/kobo/download: %v, URL: %s, Params: %v", bookmarkFound.ID, err, r.URL.Path, r.URL.Query())
		return
	}

	doc, err := html.Parse(strings.NewReader(articleHTML))
	if err != nil {
		http.Error(w, "Failed to parse article HTML", http.StatusInternalServerError)
		log.Printf("Error parsing article HTML for bookmark %s in /api/kobo/download: %v, URL: %s, Params: %v", bookmarkFound.ID, err, r.URL.Path, r.URL.Query())
		return
	}

	images := make(map[string]any)
	var imageIndex int
	var processNode func(*html.Node)
	processNode = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "img" {
			for _, attr := range n.Attr {
				if attr.Key == "src" {
					src := attr.Val
					images[fmt.Sprintf("%d", imageIndex)] = map[string]any{
						"image_id": fmt.Sprintf("%d", imageIndex),
						"item_id":  fmt.Sprintf("%d", imageIndex),
						"src":      src,
					}
					comment := &html.Node{
						Type: html.CommentNode,
						Data: fmt.Sprintf("IMG_%d", imageIndex),
					}
					if n.Parent != nil {
						n.Parent.InsertBefore(comment, n)
						n.Parent.RemoveChild(n)
					}
					imageIndex++
					break
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			processNode(c)
		}
	}
	processNode(doc)

	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		http.Error(w, "Failed to render modified HTML", http.StatusInternalServerError)
		log.Printf("Error rendering modified HTML for bookmark %s in /api/kobo/download: %v, URL: %s, Params: %v", bookmarkFound.ID, err, r.URL.Path, r.URL.Query())
		return
	}

	response := map[string]any{
		"images":  images,
		"article": buf.String(),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response for /api/kobo/download: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
	}
}

func getSitesToTry(host string) []string {
	var sites []string
	parts := strings.Split(host, ".")

	// Always try the full host first
	sites = append(sites, host)

	// If there are at least two parts, try the second-to-last part (e.g., "theatlantic" from "www.theatlantic.com")
	if len(parts) >= 2 {
		siteName := parts[len(parts)-2]
		if siteName != "" && siteName != host {
			sites = append(sites, siteName)
		}
	}

	// Remove duplicates if any (e.g., if host was "example.com", siteName would be "example", both are valid)
	uniqueSites := make([]string, 0, len(sites))
	seen := make(map[string]bool)
	for _, site := range sites {
		if _, ok := seen[site]; !ok {
			seen[site] = true
			uniqueSites = append(uniqueSites, site)
		}
	}

	return uniqueSites
}

// ExistingItemAction represents an action on an existing item.
type ExistingItemAction struct {
	Action string `json:"action"`
	ItemID string `json:"item_id"`
}

// NewItemAction represents an action to create a new item.
type NewItemAction struct {
	Action string `json:"action"`
	URL    string `json:"url"`
}

// SendRequest represents the incoming request for /api/kobo/send
type SendRequest struct {
	AccessToken string `json:"access_token"`
	ConsumerKey string `json:"consumer_key"`
	Actions     []any  `json:"actions"`
}

// HandleKoboSend handles the /api/kobo/send endpoint.
func (a *App) HandleKoboSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		log.Printf("Error decoding /api/kobo/send request: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
		return
	}

	ctx := r.Context()
	actionResults := make([]bool, len(req.Actions))
	allSucceeded := true

	for i, actionInterface := range req.Actions {
		actionMap, ok := actionInterface.(map[string]any)
		if !ok {
			actionResults[i] = false
			allSucceeded = false
			continue
		}

		action, _ := actionMap["action"].(string)
		var err error

		switch action {
		case "archive":
			itemID, _ := actionMap["item_id"].(string)
			err = a.ReadeckClient.UpdateBookmark(ctx, itemID, map[string]any{"is_archived": true})
		case "readd":
			itemID, _ := actionMap["item_id"].(string)
			err = a.ReadeckClient.UpdateBookmark(ctx, itemID, map[string]any{"is_archived": false})
		case "favorite":
			itemID, _ := actionMap["item_id"].(string)
			err = a.ReadeckClient.UpdateBookmark(ctx, itemID, map[string]any{"is_marked": true})
		case "unfavorite":
			itemID, _ := actionMap["item_id"].(string)
			err = a.ReadeckClient.UpdateBookmark(ctx, itemID, map[string]any{"is_marked": false})
		case "delete":
			itemID, _ := actionMap["item_id"].(string)
			err = a.ReadeckClient.UpdateBookmark(ctx, itemID, map[string]any{"is_deleted": true})
		case "add":
			url, _ := actionMap["url"].(string)
			err = a.ReadeckClient.CreateBookmark(ctx, url)
		default:
			err = fmt.Errorf("unknown action: %s", action)
		}

		if err != nil {
			log.Printf("Error processing action '%s' in /api/kobo/send: %v, URL: %s, Params: %v", action, err, r.URL.Path, r.URL.Query())
			actionResults[i] = false
			allSucceeded = false
		} else {
			actionResults[i] = true
		}
	}

	response := map[string]any{
		"status":         allSucceeded,
		"action_results": actionResults,
	}

	w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				log.Printf("Error encoding response for /api/kobo/send: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
			}}

// HandleConvertImage handles the /api/convert-image endpoint.
func (a *App) HandleConvertImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	imageURL := r.URL.Query().Get("url")
	if imageURL == "" {
		http.Error(w, "Missing 'url' parameter", http.StatusBadRequest)
		return
	}

	resp, err := http.Get(imageURL)
	if err != nil {
		log.Printf("Failed to fetch image %s in /api/convert-image: %v, URL: %s, Params: %v", imageURL, err, r.URL.Path, r.URL.Query())
		a.returnPlaceholderImage(w, r, "Image fetch failed")
		return
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Error closing response body for image %s in /api/convert-image: %v, URL: %s, Params: %v", imageURL, err, r.URL.Path, r.URL.Query())
		}
	}()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Failed to fetch image %s in /api/convert-image: status %d, URL: %s, Params: %v", imageURL, resp.StatusCode, r.URL.Path, r.URL.Query())
		a.returnPlaceholderImage(w, r, "Image not found")
		return
	}

	img, _, err := image.Decode(resp.Body)
	if err != nil {
		log.Printf("Failed to decode image %s in /api/convert-image: %v, URL: %s, Params: %v", imageURL, err, r.URL.Path, r.URL.Query())
		a.returnPlaceholderImage(w, r, "Image decoding failed")
		return
	}

	b := img.Bounds()
	rgbImg := image.NewRGBA(b)
	draw.Draw(rgbImg, b, img, image.Point{}, draw.Src)

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=3600")
			if err := jpeg.Encode(w, rgbImg, &jpeg.Options{Quality: 85}); err != nil {
				log.Printf("Failed to encode JPEG for image %s in /api/convert-image: %v, URL: %s, Params: %v", imageURL, err, r.URL.Path, r.URL.Query())
			}}

func (a *App) returnPlaceholderImage(w http.ResponseWriter, r *http.Request, message string) {
	width, height := 800, 600
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), image.White, image.Point{}, draw.Src)

	col := image.Black
	point := fixed.Point26_6{X: fixed.Int26_6(20 * 64), Y: fixed.Int26_6(300 * 64)}
	d := &font.Drawer{
		Dst:  img,
		Src:  col,
		Face: basicfont.Face7x13,
		Dot:  point,
	}
	d.DrawString(message)

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-control", "public, max-age=300")
			if err := jpeg.Encode(w, img, &jpeg.Options{Quality: 85}); err != nil {
				log.Printf("Error encoding placeholder image: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
			}}