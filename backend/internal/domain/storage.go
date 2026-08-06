package domain

import "context"

// StorageType is the physical backing of a storage volume (ADR-011).
type StorageType string

const (
	StorageTypeInternal StorageType = "internal"
	StorageTypeExternal StorageType = "external"
	StorageTypeNetwork  StorageType = "network"
)

// Storage is a configured media source volume (ADR-011 / ADR-014).
type Storage struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Type      StorageType `json:"type"`
	RootPath  string      `json:"root_path"`
	DeviceID  string      `json:"device_id,omitempty"`
	Readonly  bool        `json:"readonly"`
	Enabled   bool        `json:"enabled"`
	Available bool        `json:"available"`
	Busy      bool        `json:"busy"`
	CreatedAt string      `json:"created_at"`
}

// StorageRepo persists Storage records.
type StorageRepo interface {
	List(ctx context.Context) ([]Storage, error)
	Get(ctx context.Context, id string) (Storage, error)
	Create(ctx context.Context, s Storage) error
	Update(ctx context.Context, s Storage) error
	Delete(ctx context.Context, id string) error
}
