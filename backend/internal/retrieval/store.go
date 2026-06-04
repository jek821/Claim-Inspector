package retrieval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
)

// DiskCache persists chunk embeddings keyed by source URL to avoid re-embedding the same article.
type DiskCache struct {
	dir string
}

func NewDiskCache(dir string) *DiskCache {
	if dir == "" {
		return nil
	}
	return &DiskCache{dir: dir}
}

type cachedSource struct {
	URL    string          `json:"url"`
	Chunks []cachedChunk   `json:"chunks"`
}

type cachedChunk struct {
	Text      string    `json:"text"`
	Embedding []float32 `json:"embedding,omitempty"`
}

func (c *DiskCache) urlKey(url string) string {
	h := sha256.Sum256([]byte(url))
	return hex.EncodeToString(h[:16])
}

func (c *DiskCache) Load(url string) (*cachedSource, bool) {
	if c == nil {
		return nil, false
	}
	path := filepath.Join(c.dir, c.urlKey(url)+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var cs cachedSource
	if json.Unmarshal(raw, &cs) != nil || cs.URL != url {
		return nil, false
	}
	return &cs, true
}

func (c *DiskCache) Save(cs *cachedSource) error {
	if c == nil || cs == nil {
		return nil
	}
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(cs)
	if err != nil {
		return err
	}
	path := filepath.Join(c.dir, c.urlKey(cs.URL)+".json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
