package logviewer

import (
	"bufio"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// StartPruner runs PruneAll immediately and then on every tick, keeping the
// log files that back the log viewer trimmed to the last `keep` lines/entries.
func StartPruner(salesmanLogPath, flowiseLogsDir string, keep int, interval time.Duration) {
	prune := func() {
		if err := PruneAll(salesmanLogPath, flowiseLogsDir, keep); err != nil {
			log.Printf("[logviewer] prune error: %v", err)
		}
	}
	go func() {
		prune()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			prune()
		}
	}()
}

// PruneAll trims every log source used by the search/recent endpoints down to
// their last `keep` lines (or entries, for the rotated file sets), deleting
// whatever is older.
func PruneAll(salesmanLogPath, flowiseLogsDir string, keep int) error {
	if err := truncateToLastLines(salesmanLogPath, keep); err != nil {
		log.Printf("[logviewer] prune %s: %v", salesmanLogPath, err)
	}
	if flowiseLogsDir == "" {
		return nil
	}
	if err := truncateToLastLines(filepath.Join(flowiseLogsDir, "go-telegram-error.log"), keep); err != nil {
		log.Printf("[logviewer] prune go-telegram-error.log: %v", err)
	}
	if err := pruneRotated(flowiseLogsDir, "server.log.*", keep); err != nil {
		log.Printf("[logviewer] prune server.log.*: %v", err)
	}
	if err := pruneRotated(flowiseLogsDir, "audit-*.log.jsonl", keep); err != nil {
		log.Printf("[logviewer] prune audit-*.log.jsonl: %v", err)
	}
	return nil
}

// truncateToLastLines rewrites path so it contains only its last `keep`
// lines. No-op if the file is missing or already has <= keep lines.
func truncateToLastLines(path string, keep int) error {
	lines, err := readLines(path)
	if err != nil || len(lines) <= keep {
		return err
	}
	return replaceFile(path, joinLines(lines[len(lines)-keep:]))
}

// replaceFile removes path (if present) before writing fresh content. Files
// rotated in by the (root-owned) Flowise process may not be writable in
// place by this container's nonroot user, but the mounted directory is —
// unlink+create only needs directory write permission, not ownership of the
// old inode.
func replaceFile(path, content string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(path, []byte(content), 0666)
}

// pruneRotated keeps only as many of the newest files matching pattern as
// needed to cover `keep` total lines, deletes the rest, and trims the
// boundary file so the kept total is exactly `keep`.
func pruneRotated(dir, pattern string, keep int) error {
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil || len(matches) == 0 {
		return err
	}
	// Newest first — hourly-suffixed filenames sort chronologically as strings.
	sort.Sort(sort.Reverse(sort.StringSlice(matches)))

	total := 0
	keepFiles := 0
	for _, f := range matches {
		n, err := countLines(f)
		if err != nil {
			continue
		}
		total += n
		keepFiles++
		if total >= keep {
			break
		}
	}

	// Delete everything older than the files we're keeping.
	for _, f := range matches[keepFiles:] {
		if err := os.Remove(f); err != nil {
			log.Printf("[logviewer] remove %s: %v", f, err)
		}
	}

	// Trim the oldest *kept* file so the combined total is exactly `keep`.
	if keepFiles > 0 && total > keep {
		oldest := matches[keepFiles-1]
		excess := total - keep
		lines, err := readLines(oldest)
		if err != nil {
			return err
		}
		if excess >= len(lines) {
			return os.Remove(oldest)
		}
		return replaceFile(oldest, joinLines(lines[excess:]))
	}
	return nil
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

func countLines(path string) (int, error) {
	lines, err := readLines(path)
	return len(lines), err
}

func joinLines(lines []string) string {
	out := ""
	for _, l := range lines {
		out += l + "\n"
	}
	return out
}
