package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

// ItemStatus represents the current state of a download item.
type ItemStatus string

const (
	StatusPending     ItemStatus = "pending"
	StatusDownloading ItemStatus = "downloading"
	StatusExtracting  ItemStatus = "extracting"
	StatusOrganizing  ItemStatus = "organizing"
	StatusDone        ItemStatus = "done"
	StatusError       ItemStatus = "error"
)

// QueueItem represents a single download task.
type QueueItem struct {
	ID        string     `json:"id"`
	URL       string     `json:"url"`
	Status    ItemStatus `json:"status"`
	Error     string     `json:"error,omitempty"`
	AddedAt   time.Time  `json:"added_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	FileName  string     `json:"file_name,omitempty"`
	Files     []string   `json:"files,omitempty"`
}

// Queue is a thread-safe download queue backed by a JSON file.
type Queue struct {
	mu        sync.RWMutex
	items     []*QueueItem
	stateFile string
	notify    chan struct{}
}

func newQueue(stateFile string) *Queue {
	return &Queue{
		stateFile: stateFile,
		notify:    make(chan struct{}, 1),
	}
}

// newID generates a random hex identifier.
func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x", b)
}

// load reads persisted queue state from disk and resets in-progress items.
func (q *Queue) load() error {
	data, err := os.ReadFile(q.stateFile)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var items []*QueueItem
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}
	// Reset interrupted items to pending so they are retried.
	for _, item := range items {
		switch item.Status {
		case StatusDownloading, StatusExtracting, StatusOrganizing:
			item.Status = StatusPending
			item.Error = ""
			item.UpdatedAt = time.Now()
		}
	}
	q.items = items
	return nil
}

// save writes the current queue to disk (must be called with lock held or at safe point).
func (q *Queue) save() {
	data, err := json.MarshalIndent(q.items, "", "  ")
	if err != nil {
		log.Printf("queue: failed to marshal state: %v", err)
		return
	}
	if err := os.WriteFile(q.stateFile, data, 0600); err != nil {
		log.Printf("queue: failed to save state: %v", err)
	}
}

// add enqueues a new URL.
func (q *Queue) add(url string) *QueueItem {
	item := &QueueItem{
		ID:        newID(),
		URL:       url,
		Status:    StatusPending,
		AddedAt:   time.Now(),
		UpdatedAt: time.Now(),
	}
	q.mu.Lock()
	q.items = append(q.items, item)
	q.save()
	q.mu.Unlock()
	// Non-blocking notify
	select {
	case q.notify <- struct{}{}:
	default:
	}
	return item
}

// remove deletes an item by ID (only if not actively in progress).
func (q *Queue) remove(id string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, item := range q.items {
		if item.ID == id {
			q.items = append(q.items[:i], q.items[i+1:]...)
			q.save()
			return
		}
	}
}

// retry resets an errored item back to pending.
func (q *Queue) retry(id string) {
	q.mu.Lock()
	for _, item := range q.items {
		if item.ID == id && item.Status == StatusError {
			item.Status = StatusPending
			item.Error = ""
			item.UpdatedAt = time.Now()
			q.save()
			break
		}
	}
	q.mu.Unlock()
	select {
	case q.notify <- struct{}{}:
	default:
	}
}

// nextPending returns the first pending item and marks it as downloading.
func (q *Queue) nextPending() *QueueItem {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, item := range q.items {
		if item.Status == StatusPending {
			item.Status = StatusDownloading
			item.UpdatedAt = time.Now()
			q.save()
			return item
		}
	}
	return nil
}

// update changes the status and optional fields of an item.
func (q *Queue) update(id string, status ItemStatus, opts ...func(*QueueItem)) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, item := range q.items {
		if item.ID == id {
			item.Status = status
			item.UpdatedAt = time.Now()
			for _, opt := range opts {
				opt(item)
			}
			q.save()
			return
		}
	}
}

// withError is a QueueItem option that sets the error message.
func withError(msg string) func(*QueueItem) {
	return func(item *QueueItem) { item.Error = msg }
}

// withFileName is a QueueItem option that sets the file name.
func withFileName(name string) func(*QueueItem) {
	return func(item *QueueItem) { item.FileName = name }
}

// withFiles is a QueueItem option that sets the organized file list.
func withFiles(files []string) func(*QueueItem) {
	return func(item *QueueItem) { item.Files = files }
}

// getAll returns a snapshot of all queue items.
func (q *Queue) getAll() []*QueueItem {
	q.mu.RLock()
	defer q.mu.RUnlock()
	out := make([]*QueueItem, len(q.items))
	copy(out, q.items)
	return out
}

// hasPending reports whether any item is pending.
func (q *Queue) hasPending() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	for _, item := range q.items {
		if item.Status == StatusPending {
			return true
		}
	}
	return false
}
