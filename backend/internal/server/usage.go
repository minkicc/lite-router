package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type usageRecord struct {
	ID               int    `json:"id"`
	Time             int64  `json:"time"`
	TokenID          string `json:"token_id,omitempty"`
	TokenName        string `json:"token_name,omitempty"`
	Model            string `json:"model"`
	UpstreamModel    string `json:"upstream_model"`
	ChannelID        string `json:"channel_id,omitempty"`
	ChannelName      string `json:"channel_name,omitempty"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	ElapsedMS        int64  `json:"elapsed_ms"`
	Success          bool   `json:"success"`
	Endpoint         string `json:"endpoint"`
	Error            string `json:"error,omitempty"`
}

type usageStore struct {
	mu       sync.Mutex
	path     string
	max      int
	nextID   int
	records  []usageRecord
	watchers map[chan struct{}]struct{}
}

func newUsageStore(cfgPath string, maxRecords int) *usageStore {
	if maxRecords <= 0 {
		maxRecords = 500
	}
	u := &usageStore{
		path:     filepath.Join(filepath.Dir(cfgPath), "usage.json"),
		max:      maxRecords,
		nextID:   1,
		records:  []usageRecord{},
		watchers: make(map[chan struct{}]struct{}),
	}
	u.load()
	return u
}

func (u *usageStore) load() {
	data, err := os.ReadFile(u.path)
	if err != nil {
		return
	}
	var out struct {
		NextID  int           `json:"next_id"`
		Records []usageRecord `json:"records"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return
	}
	u.records = out.Records
	if u.records == nil {
		u.records = []usageRecord{}
	}
	if len(u.records) > u.max {
		u.records = u.records[len(u.records)-u.max:]
	}
	u.nextID = out.NextID
	if u.nextID <= 0 {
		u.nextID = 1
		for _, r := range u.records {
			if r.ID >= u.nextID {
				u.nextID = r.ID + 1
			}
		}
	}
}

func (u *usageStore) add(rec usageRecord) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if rec.Time == 0 {
		rec.Time = time.Now().Unix()
	}
	rec.ID = u.nextID
	u.nextID++
	u.records = append(u.records, rec)
	if len(u.records) > u.max {
		u.records = u.records[len(u.records)-u.max:]
	}
	u.persist()
	for watcher := range u.watchers {
		select {
		case watcher <- struct{}{}:
		default:
		}
	}
}

func (u *usageStore) subscribe() (<-chan struct{}, func()) {
	watcher := make(chan struct{}, 1)
	u.mu.Lock()
	u.watchers[watcher] = struct{}{}
	u.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			u.mu.Lock()
			delete(u.watchers, watcher)
			close(watcher)
			u.mu.Unlock()
		})
	}
	return watcher, cancel
}

func (u *usageStore) persist() {
	data, err := json.MarshalIndent(struct {
		NextID  int           `json:"next_id"`
		Records []usageRecord `json:"records"`
	}{NextID: u.nextID, Records: u.records}, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(u.path, data, 0600)
}

func (u *usageStore) list() []usageRecord {
	u.mu.Lock()
	defer u.mu.Unlock()
	out := make([]usageRecord, len(u.records))
	copy(out, u.records)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func (u *usageStore) summary() map[string]any {
	u.mu.Lock()
	defer u.mu.Unlock()
	var requests, success, fail int
	var prompt, completion int64
	for _, rec := range u.records {
		requests++
		if rec.Success {
			success++
		} else {
			fail++
		}
		prompt += rec.PromptTokens
		completion += rec.CompletionTokens
	}
	return map[string]any{
		"request_count":     requests,
		"success_count":     success,
		"fail_count":        fail,
		"prompt_tokens":     prompt,
		"completion_tokens": completion,
	}
}
