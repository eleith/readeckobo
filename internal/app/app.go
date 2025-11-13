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

type App struct {
	Config            *config.Config
	Logger            *logger.Logger
	ImageHTTPClient   *http.Client
	ReadeckHTTPClient *http.Client
}

func WithImageHTTPClient(client *http.Client) Option {
	return func(a *App) {
		a.ImageHTTPClient = client
	}
}

type Option func(*App)

func NewApp(opts ...Option) *App {
	app := &App{}
	for _, opt := range opts {
		opt(app)
	}
	return app
}

func WithConfig(cfg *config.Config) Option {
	return func(a *App) {
		a.Config = cfg
	}
}

func WithLogger(logger *logger.Logger) Option {
	return func(a *App) {
		a.Logger = logger
	}
}

func WithReadeckHTTPClient(client *http.Client) Option {
	return func(a *App) {
		a.ReadeckHTTPClient = client
	}
}

func (a *App) HandleKoboGet(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		a.Logger.Errorf("Error reading /api/kobo/get request body: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
		return
	}
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

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

	readeckToken, err := a.getReadeckToken(req.AccessToken)
	if err != nil {
		http.Error(w, "Invalid access token", http.StatusUnauthorized)
		a.Logger.Errorf("Error authenticating token for /api/kobo/get: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
		return
	}

	readeckClient, err := a.newReadeckClient(readeckToken)
	if err != nil {
		http.Error(w, "Failed to initialize Readeck client", http.StatusInternalServerError)
		a.Logger.Errorf("Error initializing Readeck client for /api/kobo/get: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
		return
	}

	count, _ := strconv.Atoi(req.Count)
	offset, _ := strconv.Atoi(req.Offset)

	var since *time.Time
	if req.Since != nil {
		a.Logger.Debugf("Received 'since' parameter with value: %v (type: %T)", req.Since, req.Since)
		if v, ok := req.Since.(float64); ok {
			t := time.Unix(int64(v), 0)
			since = &t
		} else {
			a.Logger.Warnf("Unexpected type for 'since' parameter: %T. Expected float64 or nil.", req.Since)
		}
	} else {
		a.Logger.Debugf("Received 'since' parameter is nil (full sync).")
	}

	resultList := make(map[string]any)

	ctx := r.Context()
	bsyncs, err := readeckClient.GetBookmarksSync(ctx, since)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get bookmark syncs: %v", err), http.StatusInternalServerError)
		a.Logger.Errorf("Error getting bookmark syncs for /api/kobo/get: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
		return
	}
	a.Logger.Debugf("HandleKoboGet: GetBookmarksSync returned %d sync events.", len(bsyncs))

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

	bookmarksDetailsMap, err := readeckClient.SyncBookmarksContent(ctx, candidateBookmarkIDs)
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
			if len(sampleIDs) == 5 {
				break
			}
		}
		a.Logger.Debugf("Sample IDs from batch response: %v", sampleIDs)
	}

	actualBookmarks := []models.KoboArticleItem{}
	var totalNonArchivedBookmarks int

	for _, bsync := range bsyncs {
		if bsync.Type == "delete" {
			continue
		}

		bookmark, found := bookmarksDetailsMap[bsync.ID]
		if !found {
			continue
		}

		if bookmark == nil {
			a.Logger.Warnf("Bookmark details for ID %s were nil in batch response for /api/kobo/get, URL: %s, Params: %v", bsync.ID, r.URL.Path, r.URL.Query())
			continue
		}

		if !bookmark.IsArchived {
			totalNonArchivedBookmarks++
		}

		if bookmark.IsArchived {
			continue
		}

		authors := make(map[string]models.KoboAuthor)
		for _, author := range bookmark.Authors {
			authors[author] = models.KoboAuthor{AuthorID: author, Name: author}
		}

		tags := make(map[string]models.KoboTag)
		for _, label := range bookmark.Labels {
			tags[label] = models.KoboTag{ItemID: bsync.ID, Tag: label}
		}

		entry := models.KoboArticleItem{
			Authors:       authors,
			Excerpt:       bookmark.Description,
			Favorite:      "0",
			GivenTitle:    bookmark.Title,
			GivenURL:      bookmark.URL,
			HasImage:      "0",
			HasVideo:      "0",
			Image:         models.KoboImage{Src: ""},
			Images:        make(map[string]models.KoboImage),
			IsArticle:     "1",
			ItemID:        bookmark.ID,
			ResolvedID:    bookmark.ID,
			ResolvedTitle: bookmark.Title,
			ResolvedURL:   bookmark.URL,
			Status:        "0",
			Tags:          tags,
			TimeAdded:     bookmark.Created.Unix(),
			TimeRead:      0,
			TimeUpdated:   bookmark.Updated.Unix(),
			Videos:        []any{},
			WordCount:     bookmark.WordCount,
			Optional:      make(map[string]any),
		}

		if bookmark.Resources.Image != nil && bookmark.Resources.Image.Src != "" {
			entry.HasImage = "1"
			entry.Image.Src = bookmark.Resources.Image.Src
			entry.Images["1"] = models.KoboImage{
				ImageID: "1",
				ItemID:  "1",
				Src:     bookmark.Resources.Image.Src,
			}
			entry.Optional["top_image_url"] = bookmark.Resources.Image.Src
		}
		actualBookmarks = append(actualBookmarks, entry)
	}

	startIndex := offset
	endIndex := offset + count
	if count == 0 {
		endIndex = len(actualBookmarks)
	}

	if startIndex > len(actualBookmarks) {
		startIndex = len(actualBookmarks)
	}
	if endIndex > len(actualBookmarks) {
		endIndex = len(actualBookmarks)
	}

	for _, bm := range actualBookmarks[startIndex:endIndex] {
		resultList[bm.ItemID] = bm
	}

	resp := models.KoboGetResponse{
		Status: 1,
		List:   resultList,
		Total:  totalNonArchivedBookmarks,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		a.Logger.Errorf("Error encoding response for /api/kobo/get: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
	}
}

func (a *App) HandleKoboDownload(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		a.Logger.Errorf("Error reading /api/kobo/download request body: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
		return
	}
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	a.Logger.Debugf("Incoming Kobo Request for /api/kobo/download:\nMethod: %s\nURL: %s\nHeaders: %v\nBody: %s", r.Method, r.URL, r.Header, string(bodyBytes))

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.KoboDownloadRequest
	if err := json.NewDecoder(bytes.NewReader(bodyBytes)).Decode(&req); err != nil {
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

	readeckToken, err := a.getReadeckToken(req.AccessToken)
	if err != nil {
		http.Error(w, "Invalid access token", http.StatusUnauthorized)
		a.Logger.Errorf("Error authenticating token for /api/kobo/download: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
		return
	}

	readeckClient, err := a.newReadeckClient(readeckToken)
	if err != nil {
		http.Error(w, "Failed to initialize Readeck client", http.StatusInternalServerError)
		a.Logger.Errorf("Error initializing Readeck client for /api/kobo/download: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
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
		totalPages := 1

		for currentPage <= totalPages {
			isArchived := false
			bookmarks, tp, err := readeckClient.GetBookmarks(ctx, site, currentPage, &isArchived)
			if err != nil {
				a.Logger.Warnf("Error searching Readeck bookmarks for site %s, page %d in /api/kobo/download: %v, URL: %s, Params: %v", site, currentPage, err, r.URL.Path, r.URL.Query())
				break
			}
			totalPages = tp

			for i := range bookmarks {
				if bookmarks[i].URL != "" {
					match, err := compareURLs(bookmarks[i].URL, reqURLStr)
					if err != nil {
						a.Logger.Warnf("Error comparing URLs for bookmark %s in /api/kobo/download: %v, URL: %s, Params: %v", bookmarks[i].ID, err, r.URL.Path, r.URL.Query())
						continue
					}
					if match {
						bookmarkFound = &bookmarks[i]
						break
					}
				}
			}
			if bookmarkFound != nil {
				break
			}
			currentPage++
		}
		if bookmarkFound != nil {
			break
		}
	}

	if bookmarkFound == nil {
		http.Error(w, "Article not found", http.StatusNotFound)
		return
	}

	articleHTML, err := readeckClient.GetBookmarkArticle(ctx, bookmarkFound.ID)
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

	sites = append(sites, host)

	if len(parts) >= 2 {
		siteName := parts[len(parts)-2]
		if siteName != "" && siteName != host {
			sites = append(sites, siteName)
		}
	}

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

func (a *App) HandleKoboSend(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		a.Logger.Errorf("Error reading /api/kobo/send request body: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
		return
	}
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	a.Logger.Debugf("Incoming Kobo Request for /api/kobo/send:\nMethod: %s\nURL: %s\nHeaders: %v\nBody: %s", r.Method, r.URL, r.Header, string(bodyBytes))

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.KoboSendRequest
	if err := json.NewDecoder(bytes.NewReader(bodyBytes)).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		a.Logger.Errorf("Error decoding /api/kobo/send request: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
		return
	}

	readeckToken, err := a.getReadeckToken(req.AccessToken)
	if err != nil {
		http.Error(w, "Invalid access token", http.StatusUnauthorized)
		a.Logger.Errorf("Error authenticating token for /api/kobo/send: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
		return
	}

	readeckClient, err := a.newReadeckClient(readeckToken)
	if err != nil {
		http.Error(w, "Failed to initialize Readeck client", http.StatusInternalServerError)
		a.Logger.Errorf("Error initializing Readeck client for /api/kobo/send: %v, URL: %s, Params: %v", err, r.URL.Path, r.URL.Query())
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
			err = readeckClient.UpdateBookmark(ctx, itemID, map[string]any{"is_archived": true})
		case "readd":
			itemID, _ := actionMap["item_id"].(string)
			err = readeckClient.UpdateBookmark(ctx, itemID, map[string]any{"is_archived": false})
		case "favorite":
			itemID, _ := actionMap["item_id"].(string)
			err = readeckClient.UpdateBookmark(ctx, itemID, map[string]any{"is_marked": true})
		case "unfavorite":
			itemID, _ := actionMap["item_id"].(string)
			err = readeckClient.UpdateBookmark(ctx, itemID, map[string]any{"is_marked": false})
		case "delete":
			itemID, _ := actionMap["item_id"].(string)
			err = readeckClient.UpdateBookmark(ctx, itemID, map[string]any{"is_deleted": true})
		case "add":
			url, _ := actionMap["url"].(string)
			err = readeckClient.CreateBookmark(ctx, url)
		case "opened_item", "left_item":
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
		client = &http.Client{Timeout: 5 * time.Second}
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

func compareURLs(url1, url2 string) (bool, error) {
	u1, err := url.Parse(strings.TrimSpace(url1))
	if err != nil {
		return false, err
	}
	u2, err := url.Parse(strings.TrimSpace(url2))
	if err != nil {
		return false, err
	}

	u1.Host = strings.TrimPrefix(u1.Host, "www.")
	u2.Host = strings.TrimPrefix(u2.Host, "www.")

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

func (a *App) newReadeckClient(readeckToken string) (*readeck.Client, error) {
	return readeck.NewClient(a.Config.Readeck.Host, readeckToken, a.Logger, a.ReadeckHTTPClient)
}

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
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	a.Logger.Debugf("Body: %s", string(bodyBytes))

	target, err := url.Parse("https://storeapi.kobo.com")
	if err != nil {
		a.Logger.Errorf("Error parsing target URL: %v", err)
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ServeHTTP(w, r)
}

