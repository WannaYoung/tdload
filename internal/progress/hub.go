package progress

import (
	"encoding/json"
	"sync"
)

type ItemCounts struct {
	Pending     int `json:"pending,omitempty"`
	Downloading int `json:"downloading,omitempty"`
	Done        int `json:"done,omitempty"`
	Skipped     int `json:"skipped,omitempty"`
	Failed      int `json:"failed,omitempty"`
}

type Event struct {
	Type       string     `json:"type"`
	Kind       string     `json:"kind,omitempty"`
	TaskID     int64      `json:"taskId"`
	ItemID     int64      `json:"itemId,omitempty"`
	ChatID     int64      `json:"chatId,omitempty"`
	MessageID  int        `json:"messageId,omitempty"`
	Phase      string     `json:"phase"`
	Done       int        `json:"done"`
	Total      int        `json:"total"`
	DoneBytes  int64      `json:"doneBytes"`
	TotalBytes int64      `json:"totalBytes"`
	Speed      int64      `json:"speed"`
	Status     string     `json:"status,omitempty"`
	Error      string     `json:"error,omitempty"`
	Title      string     `json:"title,omitempty"`
	ItemCounts ItemCounts `json:"itemCounts,omitempty"`
}

type Hub struct {
	mu   sync.RWMutex
	subs map[chan Event]struct{}
}

func NewHub() *Hub {
	return &Hub{subs: map[chan Event]struct{}{}}
}

func (h *Hub) Subscribe(buf int) (<-chan Event, func()) {
	if buf < 8 {
		buf = 8
	}
	ch := make(chan Event, buf)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
		close(ch)
	}
}

func (h *Hub) Publish(ev Event) {
	if ev.Type == "" {
		ev.Type = "task_progress"
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs {
		select {
		case ch <- ev:
		default:
			// drop if slow consumer
		}
	}
}

func (e Event) JSON() []byte {
	b, _ := json.Marshal(e)
	return b
}
