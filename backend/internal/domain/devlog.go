package domain

import "context"

// DevLogEntry is one captured frontend log line inside an archive.
type DevLogEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Module    string `json:"module"`
	Message   string `json:"message"`
}

// DevLog is a single archived snapshot of frontend logs, submitted from the
// 开发者工具 (dev tools) page. Entries are stored as JSON so the record stays a
// self-contained, copyable payload that a developer can fetch back verbatim.
type DevLog struct {
	ID        string        `json:"id"`
	Source    string        `json:"source"`
	Note      string        `json:"note"`
	Entries   []DevLogEntry `json:"entries"`
	CreatedAt string        `json:"created_at"`
}

// DevLogSummary is the lightweight list view of an archive (no entries), used
// by the dev tools archive browser.
type DevLogSummary struct {
	ID        string `json:"id"`
	Source    string `json:"source"`
	Note      string `json:"note"`
	Count     int    `json:"count"`
	CreatedAt string `json:"created_at"`
}

// DevLogRepo persists frontend log archives (developer tool).
type DevLogRepo interface {
	Create(ctx context.Context, log *DevLog) error
	List(ctx context.Context) ([]DevLogSummary, error)
	Get(ctx context.Context, id string) (DevLog, error)
	Delete(ctx context.Context, id string) error
}
