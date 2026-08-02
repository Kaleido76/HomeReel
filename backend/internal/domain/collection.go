package domain

import "context"

// Collection is a user-defined group of videos.
type Collection struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// CollectionRepo persists collections and their membership.
type CollectionRepo interface {
	List(ctx context.Context) ([]Collection, error)
	Get(ctx context.Context, id string) (Collection, error)
	Create(ctx context.Context, name string) (Collection, error)
	Rename(ctx context.Context, id, name string) error
	Delete(ctx context.Context, id string) error
	// Videos returns the videos in a collection (most recently added first).
	Videos(ctx context.Context, collectionID string) ([]Video, error)
	AddVideo(ctx context.Context, collectionID, videoID string) error
	RemoveVideo(ctx context.Context, collectionID, videoID string) error
}
