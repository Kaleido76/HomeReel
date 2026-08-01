package events

import "sync"

// Event types published by the core pipeline (ADR-010).
const (
	VideoImported    = "video.imported"
	VideoUpdated     = "video.updated"
	VideoDeleted     = "video.deleted"
	StorageMounted   = "storage.mounted"
	StorageUnmounted = "storage.unmounted"
)

// Event carries a typed message with string-keyed data.
type Event struct {
	Type string
	Data map[string]string
}

// Bus is an in-process pub/sub. Listeners receive events on their channel;
// publishes never block or drop (slow listeners get goroutine delivery).
type Bus struct {
	mu   sync.RWMutex
	subs map[string][]chan Event
}

// New builds an event bus.
func New() *Bus {
	return &Bus{subs: map[string][]chan Event{}}
}

// Subscribe returns a buffered channel receiving events of the given types.
func (b *Bus) Subscribe(types ...string) <-chan Event {
	ch := make(chan Event, 64)
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, t := range types {
		b.subs[t] = append(b.subs[t], ch)
	}
	return ch
}

// Publish delivers an event to all subscribers of its type.
func (b *Bus) Publish(ev Event) {
	b.mu.RLock()
	chans := b.subs[ev.Type]
	b.mu.RUnlock()
	for _, ch := range chans {
		select {
		case ch <- ev:
		default:
			go func(c chan Event) { c <- ev }(ch)
		}
	}
}
