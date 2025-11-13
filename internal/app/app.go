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
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
	"golang.org/x/net/html"
	"readeckobo/internal/config"
	"readeckobo/internal/logger"
	"readeckobo/internal/models"
	"readeckobo/internal/readeck"
)

// App holds the application's core dependencies and configuration.
type App struct {
	Config        *config.Config
	Logger        *logger.Logger
	ImageHTTPClient *http.Client // New field for image fetching
	ReadeckHTTPClient *http.Client // New field for Readeck API HTTP client
}

// WithImageHTTPClient sets the HTTP client for image fetching.
func WithImageHTTPClient(client *http.Client) Option {
	return func(a *App) {
		a.ImageHTTPClient = client
	}
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

// WithLogger sets the application logger.
func WithLogger(logger *logger.Logger) Option {
	return func(a *App) {
		a.Logger = logger
	}
}

// WithReadeckHTTPClient sets the HTTP client for Readeck API calls.
func WithReadeckHTTPClient(client *http.Client) Option {
	return func(a *App) {
		a.ReadeckHTTPClient = client
	}
}





	// HandleKoboGet handles the /api/kobo/get endpoint.
func (a *App) HandleKoboGet(w http.ResponseWriter, r *http.Request) {
	// Read the body once at the beginning.
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		a.Logger.Errorf("Error reading /api/kobo/get request body: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
		return
	}
	// Immediately restore the body so it can be read again by the JSON decoder.
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// Log the incoming Kobo request details. The logger's Debugf method will handle the level check.
	a.Logger.Debugf("Incoming Kobo Request for /api/kobo/get:\nMethod: %s\nURL: %s\nHeaders: %v\nBody: %s", r.Method, r.URL, r.Header, string(bodyBytes))

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.KoboGetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		a.Logger.Errorf("Error decoding /api/kobo/get request: %v, body: %s, URL: %s, Params: %v", err, string(bodyBytes), r.URL.Path, r.URL.Query())
		return
	}

	// Authenticate the request by looking up the provided token
	readeckToken, err := a.getReadeckToken(req.AccessToken)
	if err != nil {
		http.Error(w, "Invalid access token", http.StatusUnauthorized)
		a.Logger.Errorf("Error authenticating token for /api/kobo/get: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
		return
	}

	readeckClient, err := readeck.NewClient(a.Config.Readeck.Host, readeckToken, a.Logger, a.ReadeckHTTPClient)
	if err != nil {
		http.Error(w, "Failed to initialize Readeck client", http.StatusInternalServerError)
		a.Logger.Errorf("Error initializing Readeck client with looked-up token for /api/kobo/get: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
		return
	}

	count, _ := strconv.Atoi(req.Count)
	offset, _ := strconv.Atoi(req.Offset)

	var since *time.Time
	if req.Since != nil {
		a.Logger.Debugf("Received 'since' parameter with value: %v (type: %T)", req.Since, req.Since)
		if v, ok := req.Since.(float64); ok { // JSON numbers are decoded as float64
			t := time.Unix(int64(v), 0)
			since = &t
		} else {
			a.Logger.Warnf("Unexpected type for 'since' parameter: %T. Expected float64 or nil.", req.Since)
		}
	} else {
		a.Logger.Debugf("Received 'since' parameter is nil (full sync).")
	}

	resultList := make(map[string]any) // Declare resultList here

	ctx := r.Context()
	bsyncs, err := readeckClient.GetBookmarksSync(ctx, since) // Use the new client
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get bookmark syncs: %v", err), http.StatusInternalServerError)
		a.Logger.Errorf("Error getting bookmark syncs for /api/kobo/get: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
		return
	}
	a.Logger.Debugf("HandleKoboGet: GetBookmarksSync returned %d sync events.", len(bsyncs))

	// Filter out deleted bookmarks and collect IDs for fetching details
	var candidateBookmarkIDs []string
	for _, bsync := range bsyncs {
		if bsync.Type == "delete" {
			resultList[bsync.ID] = map[string]any{
				"item_id": bsync.ID,
				"status":  "2",
			}
		} else {
			candidateBookmarkIDs = append(candidateBookmarkIDs, bsync.ID)
		}
	}
	a.Logger.Debugf("HandleKoboGet: %d candidate bookmark IDs identified for batch fetching.", len(candidateBookmarkIDs))

	// Fetch all candidate bookmark details in a single batch call
	bookmarksDetailsMap, err := readeckClient.SyncBookmarksContent(ctx, candidateBookmarkIDs) // Use the new client
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get bookmark details in batch: %v", err), http.StatusInternalServerError)
		a.Logger.Errorf("Error getting bookmark details in batch for /api/kobo/get: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
		return
	}

	a.Logger.Debugf("SyncBookmarksContent returned %d bookmark details.", len(bookmarksDetailsMap))
	if len(bookmarksDetailsMap) > 0 {
		sampleIDs := make([]string, 0, 5)
		for id := range bookmarksDetailsMap {
			sampleIDs = append(sampleIDs, id)
			if len(sampleIDs) == 5 { break }
		}
		a.Logger.Debugf("Sample IDs from batch response: %v", sampleIDs)
	}

	actualBookmarks := []map[string]any{} // To store processed, non-archived bookmarks in order
	var totalNonArchivedBookmarks int

	// Iterate through the original sync events to maintain order and apply filtering
	for _, bsync := range bsyncs {
		if bsync.Type == "delete" {
			continue // Already handled, or not relevant for actualBookmarks
		}

		bookmark, found := bookmarksDetailsMap[bsync.ID]
		if !found {
			// a.Logger.Warnf("Bookmark details for ID %s not found in batch response for /api/kobo/get, URL: %s, Params: %v", bsync.ID, r.URL.Path, r.URL.Query())
			continue
		}

		if bookmark == nil { // Should not happen if 'found' is true, but good for safety
			a.Logger.Warnf("Bookmark details for ID %s were nil in batch response for /api/kobo/get, URL: %s, Params: %v", bsync.ID, r.URL.Path, r.URL.Query())
			continue
		}

		// Count all non-archived bookmarks for the total field
		if !bookmark.IsArchived {
			totalNonArchivedBookmarks++
		}

		// Only add non-archived bookmarks to actualBookmarks for pagination
		if bookmark.IsArchived {
			continue
		}

		// Construct the bookmark entry for resultList
		entry := make(map[string]any)
		entry["authors"] = make(map[string]any)
		for _, author := range bookmark.Authors {
			entry["authors"].(map[string]any)[author] = map[string]string{"author_id": author, "name": author}
		}
		entry["excerpt"] = bookmark.Description
		entry["favorite"] = "0"
		entry["given_title"] = bookmark.Title
		entry["given_url"] = bookmark.URL
		entry["has_image"] = "0"
		entry["has_video"] = "0"
		entry["image"] = map[string]any{"src": ""}
		entry["images"] = map[string]any{}
		entry["is_article"] = "1"
		entry["item_id"] = bookmark.ID
		entry["resolved_id"] = bookmark.ID
		entry["resolved_title"] = bookmark.Title
		entry["resolved_url"] = bookmark.URL
		entry["status"] = "0"
		entry["tags"] = make(map[string]any)
		for _, label := range bookmark.Labels {
			entry["tags"].(map[string]any)[label] = map[string]string{"item_id": bsync.ID, "tag": label}
		}
		entry["time_added"] = bookmark.Created.Unix()
		entry["time_read"] = 0
		entry["time_updated"] = bookmark.Updated.Unix()
		entry["videos"] = []any{}
		entry["word_count"] = bookmark.WordCount
		entry["_optional"] = make(map[string]any)

		if bookmark.Resources.Image != nil && bookmark.Resources.Image.Src != "" {
			entry["has_image"] = "1"
			entry["image"].(map[string]any)["src"] = bookmark.Resources.Image.Src
			entry["images"].(map[string]any)["1"] = map[string]any{
				"image_id": "1",
				"item_id":  "1",
				"src":      bookmark.Resources.Image.Src,
			}
			entry["_optional"].(map[string]any)["top_image_url"] = bookmark.Resources.Image.Src
		}
		actualBookmarks = append(actualBookmarks, entry)
	}

	// Apply Kobo client's count and offset logic to the filtered actualBookmarks
	startIndex := offset
	endIndex := offset + count
	if count == 0 { // If count is 0, return all
		endIndex = len(actualBookmarks)
	}

	if startIndex > len(actualBookmarks) {
		startIndex = len(actualBookmarks)
	}
	if endIndex > len(actualBookmarks) {
		endIndex = len(actualBookmarks)
	}

	// Populate resultList with the paginated and filtered bookmarks
	for _, bm := range actualBookmarks[startIndex:endIndex] {
		resultList[bm["item_id"].(string)] = bm
	}

	resp := models.KoboGetResponse{
		Status: 1,
		List:   resultList,
		Total:  totalNonArchivedBookmarks, // Total number of non-archived bookmarks
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		a.Logger.Errorf("Error encoding response for /api/kobo/get: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
	}
}


// HandleKoboDownload handles the /api/kobo/download endpoint.
func (a *App) HandleKoboDownload(w http.ResponseWriter, r *http.Request) {
	// Read the body once at the beginning.
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		a.Logger.Errorf("Error reading /api/kobo/download request body: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
		return
	}
	// Immediately restore the body so it can be read again by the JSON decoder or form parser.
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// Log the incoming Kobo request details. The logger's Debugf method will handle the level check.
	a.Logger.Debugf("Incoming Kobo Request for /api/kobo/download:\nMethod: %s\nURL: %s\nHeaders: %v\nBody: %s", r.Method, r.URL, r.Header, string(bodyBytes))

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.KoboDownloadRequest
	// Use the restored body for decoding.
	if err := json.NewDecoder(bytes.NewReader(bodyBytes)).Decode(&req); err != nil {
		// If JSON decoding fails, try form parsing (Kobo devices might send form data for download)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid request body or form data", http.StatusBadRequest)
			a.Logger.Errorf("Error decoding /api/kobo/download request: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
			return
		}
		req.AccessToken = r.FormValue("access_token")
		req.ConsumerKey = r.FormValue("consumer_key")
		req.Images, _ = strconv.Atoi(r.FormValue("images"))
		req.Refresh, _ = strconv.Atoi(r.FormValue("refresh"))
		req.Output = r.FormValue("output")
		req.URL = r.FormValue("url")
	}

	// Authenticate the request by looking up the provided token
	readeckToken, err := a.getReadeckToken(req.AccessToken)
	if err != nil {
		http.Error(w, "Invalid access token", http.StatusUnauthorized)
		a.Logger.Errorf("Error authenticating token for /api/kobo/download: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
		return
	}

	// Create a new Readeck client with the looked-up token for this request
	readeckClient, err := readeck.NewClient(a.Config.Readeck.Host, readeckToken, a.Logger, a.ReadeckHTTPClient)
	if err != nil {
		http.Error(w, "Failed to initialize Readeck client", http.StatusInternalServerError)
		a.Logger.Errorf("Error initializing Readeck client with looked-up token for /api/kobo/download: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
		return
	}

	reqURLStr := req.URL
	if reqURLStr == "" {
		http.Error(w, "Missing 'url' parameter", http.StatusBadRequest)
		a.Logger.Errorf("Error: Missing 'url' parameter in /api/kobo/download request, URL: %s, Params: %v", r.URL.Path, r.URL.Query())
		return
	}

	parsedURL, err := url.Parse(reqURLStr)
	if err != nil {
		http.Error(w, "Invalid 'url' parameter", http.StatusBadRequest)
		a.Logger.Errorf("Error: Invalid 'url' parameter in /api/kobo/download request: %v, url: %s, URL: %s, Params: %v", err, reqURLStr, r.URL.Path, r.URL.Query())
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
			bookmarks, tp, err := readeckClient.GetBookmarks(ctx, site, currentPage, &isArchived) // Use the new client
			if err != nil {
				a.Logger.Warnf("Error searching Readeck bookmarks for site %s, page %d in /api/kobo/download: %v, URL: %s, Params: %v", site, currentPage, err, r.URL.Path, r.URL.Query())
				break // Break from inner loop, try next site
			}
			totalPages = tp // Update totalPages from the response header

			for i := range bookmarks {
				if bookmarks[i].URL != "" {
					match, err := compareURLs(bookmarks[i].URL, reqURLStr)
					if err != nil {
						a.Logger.Warnf("Error comparing URLs for bookmark %s in /api/kobo/download: %v, URL: %s, Params: %v", bookmarks[i].ID, err, r.URL.Path, r.URL.Query())
						continue
					}
					if match {
						bookmarkFound = &bookmarks[i]
						break // Found the bookmark, break from inner loop
					}
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

	articleHTML, err := readeckClient.GetBookmarkArticle(ctx, bookmarkFound.ID) // Use the new client
	if err != nil {
		http.Error(w, "Failed to fetch article content", http.StatusInternalServerError)
		a.Logger.Errorf("Error fetching article content for bookmark %s in /api/kobo/download: %v, URL: %s, Params: %v", bookmarkFound.ID, err, r.URL.Path, r.URL.Query())
		return
	}

	doc, err := html.Parse(strings.NewReader(articleHTML))
	if err != nil {
		http.Error(w, "Failed to parse article HTML", http.StatusInternalServerError)
		a.Logger.Errorf("Error parsing article HTML for bookmark %s in /api/kobo/download: %v, URL: %s, Params: %v", bookmarkFound.ID, err, r.URL.Path, r.URL.Query())
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
		a.Logger.Errorf("Error rendering modified HTML for bookmark %s in /api/kobo/download: %v, URL: %s, Params: %v", bookmarkFound.ID, err, r.URL.Path, r.URL.Query())
		return
	}

	response := map[string]any{
		"images":  images,
		"article": buf.String(),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		a.Logger.Errorf("Error encoding response for /api/kobo/download: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
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



// HandleKoboSend handles the /api/kobo/send endpoint.
func (a *App) HandleKoboSend(w http.ResponseWriter, r *http.Request) {
	// Read the body once at the beginning.
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		a.Logger.Errorf("Error reading /api/kobo/send request body: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
		return
	}
	// Immediately restore the body so it can be read again by the JSON decoder.
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// Log the incoming Kobo request details. The logger's Debugf method will handle the level check.
	a.Logger.Debugf("Incoming Kobo Request for /api/kobo/send:\nMethod: %s\nURL: %s\nHeaders: %v\nBody: %s", r.Method, r.URL, r.Header, string(bodyBytes))

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.KoboSendRequest
	// Use the restored body for decoding.
	if err := json.NewDecoder(bytes.NewReader(bodyBytes)).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		a.Logger.Errorf("Error decoding /api/kobo/send request: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
		return
	}
	
		// Authenticate the request by looking up the provided token
		readeckToken, err := a.getReadeckToken(req.AccessToken)
		if err != nil {
			http.Error(w, "Invalid access token", http.StatusUnauthorized)
			a.Logger.Errorf("Error authenticating token for /api/kobo/send: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
			return
		}
	
			// Create a new Readeck client with the looked-up token for this request
			readeckClient, err := readeck.NewClient(a.Config.Readeck.Host, readeckToken, a.Logger, a.ReadeckHTTPClient)
			if err != nil {		http.Error(w, "Failed to initialize Readeck client", http.StatusInternalServerError)
		a.Logger.Errorf("Error initializing Readeck client with looked-up token for /api/kobo/send: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
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
			err = readeckClient.UpdateBookmark(ctx, itemID, map[string]any{"is_archived": true}) // Use the new client
		case "readd":
			itemID, _ := actionMap["item_id"].(string)
			err = readeckClient.UpdateBookmark(ctx, itemID, map[string]any{"is_archived": false}) // Use the new client
		case "favorite":
			itemID, _ := actionMap["item_id"].(string)
			err = readeckClient.UpdateBookmark(ctx, itemID, map[string]any{"is_marked": true}) // Use the new client
		case "unfavorite":
			itemID, _ := actionMap["item_id"].(string)
			err = readeckClient.UpdateBookmark(ctx, itemID, map[string]any{"is_marked": false}) // Use the new client
		case "delete":
			itemID, _ := actionMap["item_id"].(string)
			err = readeckClient.UpdateBookmark(ctx, itemID, map[string]any{"is_deleted": true}) // Use the new client
		case "add":
			url, _ := actionMap["url"].(string)
			err = readeckClient.CreateBookmark(ctx, url) // Use the new client
		case "opened_item", "left_item":
			// Kobo sends these, but Readeck doesn't need them. No-op.
			err = nil
		default:
			err = fmt.Errorf("unknown action: %s", action)
		}

		if err != nil {
			a.Logger.Warnf("Error processing action '%s' in /api/kobo/send: %v, URL: %s, Params: %v", action, err, r.URL.Path, r.URL.Query())
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
		a.Logger.Errorf("Error encoding response for /api/kobo/send: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
	}
}

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

	client := a.ImageHTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second} // Default client with timeout
	}
	resp, err := client.Get(imageURL)
	if err != nil {
		a.Logger.Errorf("Failed to fetch image %s in /api/convert-image: %v, URL: %s, Params: %v", imageURL, err, r.URL.Path, r.URL.Query())
		a.returnPlaceholderImage(w, r, "Image fetch failed")
		return
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			a.Logger.Warnf("Error closing response body for image %s in /api/convert-image: %v, URL: %s, Params: %v", imageURL, err, r.URL.Path, r.URL.Query())
		}
	}()

	if resp.StatusCode != http.StatusOK {
		a.Logger.Warnf("Failed to fetch image %s in /api/convert-image: status %d, URL: %s, Params: %v", imageURL, resp.StatusCode, r.URL.Path, r.URL.Query())
		a.returnPlaceholderImage(w, r, "Image not found")
		return
	}

	img, _, err := image.Decode(resp.Body)
	if err != nil {
		a.Logger.Warnf("Failed to decode image %s in /api/convert-image: %v, URL: %s, Params: %v", imageURL, err, r.URL.Path, r.URL.Query())
		a.returnPlaceholderImage(w, r, "Image decoding failed")
		return
	}

	b := img.Bounds()
	rgbImg := image.NewRGBA(b)
	draw.Draw(rgbImg, b, img, image.Point{}, draw.Src)

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	if err := jpeg.Encode(w, rgbImg, &jpeg.Options{Quality: 85}); err != nil {
		a.Logger.Errorf("Failed to encode JPEG for image %s in /api/convert-image: %v, URL: %s, Params: %v", imageURL, err, r.URL.Path, r.URL.Query())
	}
}

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
		a.Logger.Errorf("Error encoding placeholder image: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
	}
}

// compareURLs robustly compares two URLs by normalizing them and ignoring query parameters and fragments.
func compareURLs(url1, url2 string) (bool, error) {
	u1, err := url.Parse(strings.TrimSpace(url1))
	if err != nil {
		return false, err
	}
	u2, err := url.Parse(strings.TrimSpace(url2))
	if err != nil {
		return false, err
	}

	// Normalize by removing 'www.' from host
	u1.Host = strings.TrimPrefix(u1.Host, "www.")
	u2.Host = strings.TrimPrefix(u2.Host, "www.")

	// Compare scheme, host, and path, but ignore query params and fragments
	return u1.Scheme == u2.Scheme && u1.Host == u2.Host && u1.Path == u2.Path, nil
}

func (a *App) getReadeckToken(deviceToken string) (string, error) {
	for _, user := range a.Config.Users {
		if user.Token == deviceToken {
			return user.ReadeckAccessToken, nil
		}
	}
	return "", fmt.Errorf("unauthorized device token")
}

// HandleDumpAndForward handles the dump and forward endpoint.
func (a *App) HandleDumpAndForward(w http.ResponseWriter, r *http.Request) {
	a.Logger.Debugf("Dumping request from %s", r.RemoteAddr)
	a.Logger.Debugf("Method: %s", r.Method)
	a.Logger.Debugf("URL: %s", r.URL.String())
	a.Logger.Debugf("Headers: %v", r.Header)

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		a.Logger.Debugf("Error reading request body: %v", err)
		return
	}
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes)) // Restore the body for subsequent reads

	a.Logger.Debugf("Body: %s", string(bodyBytes))

	// Forward the request to the real Kobo API
	target, err := url.Parse("https://storeapi.kobo.com")
	if err != nil {
		a.Logger.Errorf("Error parsing target URL: %v", err)
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ServeHTTP(w, r)
}