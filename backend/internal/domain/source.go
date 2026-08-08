package domain

import "context"

// MediaSource is a directory the user has declared as a multimedia source: a
// persistent marker plus a scan unit for the video library. It is deliberately
// lightweight — it never becomes a browsable object in the file browser and is
// not tied to any lifecycle beyond "re-scan this path". Its presence in the
// library only ever changes because the real files under it change.
type MediaSource struct {
	ID         string `json:"id"`
	Path       string `json:"path"` // normalized absolute path
	CreatedAt  string `json:"created_at"`
	LastScanAt string `json:"last_scan_at,omitempty"`
}

// SourceRepo persists MediaSource markers.
type SourceRepo interface {
	List(ctx context.Context) ([]MediaSource, error)
	Get(ctx context.Context, id string) (MediaSource, error)
	// GetByPath resolves a source by its exact normalized path.
	GetByPath(ctx context.Context, path string) (MediaSource, error)
	Create(ctx context.Context, s MediaSource) error
	Delete(ctx context.Context, id string) error
	// TouchLastScan records the last successful scan start.
	TouchLastScan(ctx context.Context, id, lastScanAt string) error
}
