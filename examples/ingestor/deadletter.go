package main

import (
	"encoding/json"
	"os"
	"sync"
)

// DeadLetter appends documents that could not be indexed to an NDJSON file.
// Nothing is lost silently: fix the cause, then replay the file.
type DeadLetter struct {
	mu  sync.Mutex
	f   *os.File
	enc *json.Encoder
}

func newDeadLetter(path string) (*DeadLetter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &DeadLetter{f: f, enc: json.NewEncoder(f)}, nil
}

func (d *DeadLetter) Write(fl failure) {
	d.mu.Lock()
	defer d.mu.Unlock()
	_ = d.enc.Encode(map[string]any{
		"id": fl.Event.ID, "status": fl.Status, "reason": fl.Reason, "doc": fl.Event,
	})
}

func (d *DeadLetter) Close() error { return d.f.Close() }
