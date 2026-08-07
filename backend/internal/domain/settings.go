package domain

import "context"

// SettingsRepo persists key/value runtime settings. The auth package reads and
// writes its password hash through the settings table; the generic file browser
// stores its pinned paths there too. Missing keys yield ErrNotFound.
type SettingsRepo interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
}
