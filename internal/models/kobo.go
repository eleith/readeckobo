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
