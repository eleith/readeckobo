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
	Status int                         `json:"status"`
	List   map[string]KoboArticleItem `json:"list"`
	Total  int                         `json:"total"`
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
	Authors       map[string]KoboAuthor `json:"authors,omitempty"`
	Excerpt       string                `json:"excerpt,omitempty"`
	Favorite      string                `json:"favorite,omitempty"`
	GivenTitle    string                `json:"given_title,omitempty"`
	GivenURL      string                `json:"given_url,omitempty"`
	HasImage      string                `json:"has_image,omitempty"`
	HasVideo      string                `json:"has_video,omitempty"`
	Image         *KoboImage            `json:"image,omitempty"`
	Images        map[string]KoboImage  `json:"images,omitempty"`
	IsArticle     string                `json:"is_article,omitempty"`
	ItemID        string                `json:"item_id"`
	ResolvedID    string                `json:"resolved_id,omitempty"`
	ResolvedTitle string                `json:"resolved_title,omitempty"`
	ResolvedURL   string                `json:"resolved_url,omitempty"`
	Status        string                `json:"status"`
	Tags          map[string]KoboTag    `json:"tags,omitempty"`
	TimeAdded     int64                 `json:"time_added,omitempty"`
	TimeRead      int64                 `json:"time_read,omitempty"`
	TimeUpdated   int64                 `json:"time_updated,omitempty"`
	Videos        []any                 `json:"videos,omitempty"`
	WordCount     int                   `json:"word_count,omitempty"`
	Optional      map[string]any        `json:"_optional,omitempty"`
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
