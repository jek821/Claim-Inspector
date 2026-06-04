// Package envload provides a minimal .env file loader.
// It reads KEY=VALUE lines and calls os.Setenv for any key not already set.
package envload

import (
	"bufio"
	"os"
	"strings"
)

// Load reads a .env file and sets any unset environment variables from it.
// Silently ignores a missing file.
func Load(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 1 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		if ci := strings.Index(val, " #"); ci != -1 {
			val = strings.TrimSpace(val[:ci])
		}
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
	return scanner.Err()
}
