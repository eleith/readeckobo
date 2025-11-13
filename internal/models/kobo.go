package models

// KoboGetRequest is the incoming request for /api/kobo/get
type KoboGetRequest struct {
	AccessToken string `json:"access_token"`
	ConsumerKey string `json:"consumer_key"`
	ContentType string `json:"contentType"`
	Count       string `json:"count"`
	DetailType  string `json:"detailType"`
	Offset      string `json:"offset"`
	State       string `json:"state"`
	Total       string `json:"total"`
	Since       any    `json:"since"`
}

// KoboGetResponse represents the outgoing response for /api/kobo/get
type KoboGetResponse struct {
	Status int            `json:"status"`
	List   map[string]any `json:"list"`
	Total  int            `json:"total"`
}

// KoboDownloadRequest represents the incoming request for /api/kobo/download
type KoboDownloadRequest struct {
	AccessToken string `json:"access_token"`
	ConsumerKey string `json:"consumer_key"`
	Images      int    `json:"images"`
	Refresh     int    `json:"refresh"`
	Output      string `json:"output"`
	URL         string `json:"url"`
}

// KoboSendRequest represents the incoming request for /api/kobo/send
type KoboSendRequest struct {
	AccessToken string `json:"access_token"`
	ConsumerKey string `json:"consumer_key"`
	Actions     []any  `json:"actions"`
}

// KoboArticleItem represents an article in the Get response list.
type KoboArticleItem struct {
	Authors       map[string]KoboAuthor `json:"authors"`
	Excerpt       string                `json:"excerpt"`
	Favorite      string                `json:"favorite"`
	GivenTitle    string                `json:"given_title"`
	GivenURL      string                `json:"given_url"`
	HasImage      string                `json:"has_image"`
	HasVideo      string                `json:"has_video"`
	Image         KoboImage             `json:"image"`
	Images        map[string]KoboImage  `json:"images"`
	IsArticle     string                `json:"is_article"`
	ItemID        string                `json:"item_id"`
	ResolvedID    string                `json:"resolved_id"`
	ResolvedTitle string                `json:"resolved_title"`
	ResolvedURL   string                `json:"resolved_url"`
	Status        string                `json:"status"`
	Tags          map[string]KoboTag    `json:"tags"`
	TimeAdded     int64                 `json:"time_added"`
	TimeRead      int64                 `json:"time_read"`
	TimeUpdated   int64                 `json:"time_updated"`
	Videos        []any                 `json:"videos"`
	WordCount     int                   `json:"word_count"`
	Optional      map[string]any        `json:"_optional"`
}

// KoboAuthor represents an author of an article.
type KoboAuthor struct {
	AuthorID string `json:"author_id"`
	Name     string `json:"name"`
}

// KoboImage represents an image associated with an article.
type KoboImage struct {
	ImageID string `json:"image_id,omitempty"`
	ItemID  string `json:"item_id,omitempty"`
	Src     string `json:"src"`
}

// KoboTag represents a tag associated with an article.
type KoboTag struct {
	ItemID string `json:"item_id"`
	Tag    string `json:"tag"`
}
