// Package store provides a simple JSON-file-backed persistent store for
// fact-check history and cumulative cost tracking.
//
// All writes are atomic: data is written to a .tmp file then renamed into
// place, so a crash mid-write never corrupts the store.
package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"factchecker/internal/types"
)

// Run is a single completed fact-check stored in history.
type Run struct {
	ID        string              `json:"id"`
	CreatedAt time.Time           `json:"created_at"`
	Title     string              `json:"title"`    // auto-generated from first ~80 chars of text
	Label     string              `json:"label"`    // user-supplied name, overrides title in UI
	FileName  string              `json:"filename"` // original upload filename if applicable
	Mode      string              `json:"mode"`     // "sync" | "batch"
	Claims    []types.Claim       `json:"claims"`
	Cost      types.CostBreakdown `json:"cost"`
}

// Data is the full on-disk structure.
type Data struct {
	Runs         []Run   `json:"runs"`
	TotalInputTok  int   `json:"total_input_tokens"`
	TotalOutputTok int   `json:"total_output_tokens"`
	TotalCostUSD float64 `json:"total_cost_usd"`
}

// Store is a thread-safe persistent JSON store.
type Store struct {
	mu   sync.RWMutex
	path string
	data Data
}

// New opens or creates the store at the given file path.
func New(path string) (*Store, error) {
	s := &Store{path: path}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// AddRun appends a completed run to history and updates cumulative cost.
func (s *Store) AddRun(run Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Keep last 200 runs
	s.data.Runs = append([]Run{run}, s.data.Runs...)
	if len(s.data.Runs) > 200 {
		s.data.Runs = s.data.Runs[:200]
	}
	s.data.TotalInputTok += run.Cost.Usage.InputTokens
	s.data.TotalOutputTok += run.Cost.Usage.OutputTokens
	s.data.TotalCostUSD += run.Cost.ExactCostUSD
	return s.save()
}

// RenameRun updates the label of a run by ID. Returns false if not found.
func (s *Store) RenameRun(id, label string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, r := range s.data.Runs {
		if r.ID == id {
			s.data.Runs[i].Label = label
			return true, s.save()
		}
	}
	return false, nil
}

// DeleteRun removes a run by ID and subtracts its cost from totals. Returns false if not found.
func (s *Store) DeleteRun(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, r := range s.data.Runs {
		if r.ID == id {
			s.data.TotalInputTok -= r.Cost.Usage.InputTokens
			s.data.TotalOutputTok -= r.Cost.Usage.OutputTokens
			s.data.TotalCostUSD -= r.Cost.ExactCostUSD
			s.data.Runs = append(s.data.Runs[:i], s.data.Runs[i+1:]...)
			return true, s.save()
		}
	}
	return false, nil
}

// GetHistory returns all stored runs (newest first).
func (s *Store) GetHistory() []Run {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Run, len(s.data.Runs))
	copy(out, s.data.Runs)
	return out
}

// GetTotals returns cumulative cost totals.
func (s *Store) GetTotals() (inputTok, outputTok int, totalCost float64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.TotalInputTok, s.data.TotalOutputTok, s.data.TotalCostUSD
}

func (s *Store) load() error {
	f, err := os.Open(s.path)
	if os.IsNotExist(err) {
		s.data = Data{}
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewDecoder(f).Decode(&s.data)
}

// save writes atomically: marshal → tmp file → rename.
func (s *Store) save() error {
	tmp := s.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s.data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, s.path)
}
