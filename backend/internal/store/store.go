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

	"factchecker/internal/apilimits"
	"factchecker/internal/types"
	"factchecker/internal/usage"
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
	APIUsage  map[string]usage.Counts `json:"api_usage,omitempty"` // per-provider deltas for this run
}

// Data is the full on-disk structure.
type Data struct {
	Runs           []Run `json:"runs"`
	TotalInputTok  int   `json:"total_input_tokens"`
	TotalOutputTok int   `json:"total_output_tokens"`
	TotalCostUSD   float64 `json:"total_cost_usd"`
	APIUsageLifetime map[string]usage.Counts `json:"api_usage_lifetime"`
	APIUsageDaily    map[string]usage.Counts `json:"api_usage_daily"`
	APIUsageDailyDate string                 `json:"api_usage_daily_date"` // UTC YYYY-MM-DD
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
	s.ensureDailyBucketLocked()
	s.applyUsageDelta(run.APIUsage, 1)
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
			s.ensureDailyBucketLocked()
			s.applyUsageDelta(r.APIUsage, -1)
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

// GetAPIUsageReport returns usage vs documented provider limits.
func (s *Store) GetAPIUsageReport() types.APIUsageResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()
	s.ensureDailyBucketLocked()
	return usage.BuildReport(s.toProviderMap(s.data.APIUsageLifetime), s.toProviderMap(s.data.APIUsageDaily), s.data.APIUsageDailyDate)
}

func (s *Store) toProviderMap(m map[string]usage.Counts) map[apilimits.Provider]usage.Counts {
	if m == nil {
		return map[apilimits.Provider]usage.Counts{}
	}
	out := make(map[apilimits.Provider]usage.Counts, len(m))
	for k, v := range m {
		out[apilimits.Provider(k)] = v
	}
	return out
}

func (s *Store) ensureDailyBucketLocked() {
	today := time.Now().UTC().Format("2006-01-02")
	if s.data.APIUsageDailyDate != today {
		s.data.APIUsageDaily = map[string]usage.Counts{}
		s.data.APIUsageDailyDate = today
	}
	if s.data.APIUsageLifetime == nil {
		s.data.APIUsageLifetime = map[string]usage.Counts{}
	}
	if s.data.APIUsageDaily == nil {
		s.data.APIUsageDaily = map[string]usage.Counts{}
	}
}

func (s *Store) applyUsageDelta(delta map[string]usage.Counts, sign int) {
	if delta == nil {
		return
	}
	for id, d := range delta {
		if sign < 0 {
			d = negateCounts(d)
		}
		s.mergeCounts(s.data.APIUsageLifetime, id, d)
		s.mergeCounts(s.data.APIUsageDaily, id, d)
	}
}

func (s *Store) mergeCounts(m map[string]usage.Counts, id string, d usage.Counts) {
	c := m[id]
	c.Requests += d.Requests
	c.InputTokens += d.InputTokens
	c.OutputTokens += d.OutputTokens
	c.EmbedTokens += d.EmbedTokens
	c.EstSpendUSD += d.EstSpendUSD
	m[id] = c
}

func negateCounts(c usage.Counts) usage.Counts {
	return usage.Counts{
		Requests:     -c.Requests,
		InputTokens:  -c.InputTokens,
		OutputTokens: -c.OutputTokens,
		EmbedTokens:  -c.EmbedTokens,
		EstSpendUSD:  -c.EstSpendUSD,
	}
}

func (s *Store) load() error {
	f, err := os.Open(s.path)
	if os.IsNotExist(err) {
		s.data = Data{
			APIUsageLifetime:  map[string]usage.Counts{},
			APIUsageDaily:     map[string]usage.Counts{},
			APIUsageDailyDate: time.Now().UTC().Format("2006-01-02"),
		}
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	if err := json.NewDecoder(f).Decode(&s.data); err != nil {
		return err
	}
	if s.data.APIUsageLifetime == nil {
		s.data.APIUsageLifetime = map[string]usage.Counts{}
	}
	if s.data.APIUsageDaily == nil {
		s.data.APIUsageDaily = map[string]usage.Counts{}
	}
	if s.data.APIUsageDailyDate == "" {
		s.data.APIUsageDailyDate = time.Now().UTC().Format("2006-01-02")
	}
	return nil
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
