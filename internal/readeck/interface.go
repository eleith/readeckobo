package readeck

import (
	"context"
	"time"
)

// ClientInterface defines the interface for the Readeck API client.
type ClientInterface interface {
	GetBookmarksSync(ctx context.Context, since *time.Time) ([]BookmarkSync, error)
	GetBookmarks(ctx context.Context, site string, page int, isArchived *bool) ([]Bookmark, int, error)
	GetBookmarkDetails(ctx context.Context, id string) (*Bookmark, error)
	GetBookmarkArticle(ctx context.Context, id string) (string, error)
	UpdateBookmark(ctx context.Context, id string, updates map[string]any) error
	CreateBookmark(ctx context.Context, bookmarkURL string) error
}
