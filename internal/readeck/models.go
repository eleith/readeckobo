package readeck

import (
	"time"
)

type BookmarkSync struct {
	ID   string    `json:"id"`
	Time time.Time `json:"time"`
	Type string    `json:"type"` // Literal["update"] | Literal["delete"]
}

type ResourceImage struct {
	Src    string `json:"src"`
	Width  int      `json:"width"`
	Height int      `json:"height"`
}

type ResourceLink struct {
	Src string `json:"src"`
}

type Resources struct {
	Article   *ResourceLink  `json:"article"`
	Icon      *ResourceImage `json:"icon"`
	Image     *ResourceImage `json:"image"`
	Log       *ResourceLink  `json:"log"`
	Props     *ResourceLink  `json:"props"`
	Thumbnail *ResourceImage `json:"thumbnail"`
}

type Bookmark struct {
	Authors      []string    `json:"authors"`
	Created      time.Time   `json:"created"`
	Description  string      `json:"description"`
	DocumentType string      `json:"document_type"`
	HasArticle   bool        `json:"has_article"`
		Href      string   `json:"href"`
	ID           string      `json:"id"`
	IsArchived   bool        `json:"is_archived"`
	IsDeleted    bool        `json:"is_deleted"`
	IsMarked     bool        `json:"is_marked"`
	Labels       []string    `json:"labels"`
	Lang         string      `json:"lang"`
	Loaded       bool        `json:"loaded"`
	ReadProgress int         `json:"read_progress"`
	Resources    Resources   `json:"resources"`
	Site         string      `json:"site"`
	SiteName     string      `json:"site_name"`
	State        int         `json:"state"`
	TextDirection string      `json:"text_direction"`
	Title        string      `json:"title"`
	Type         string      `json:"type"`
	Updated      time.Time   `json:"updated"`
		URL       string   `json:"url"`
	WordCount    *int        `json:"word_count"`
}
