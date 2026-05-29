package client

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// DownloadArchive stores completed video IDs for idempotent reruns.
type DownloadArchive struct {
	path string
	file *os.File
	mu   sync.Mutex
	ids  map[string]struct{}
}

// OpenDownloadArchive opens or creates a download archive file.
//
// Invalid/corrupt existing lines are ignored when loading, matching yt-dlp's
// forgiving archive behavior.
func OpenDownloadArchive(path string) (*DownloadArchive, error) {
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" {
		return nil, fmt.Errorf("archive path is empty")
	}
	if dir := filepath.Dir(cleanPath); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	f, err := os.OpenFile(cleanPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}

	archive := &DownloadArchive{
		path: cleanPath,
		file: f,
		ids:  make(map[string]struct{}),
	}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if _, err := ExtractVideoID(line); err != nil {
			continue
		}
		archive.ids[line] = struct{}{}
	}
	if err := scanner.Err(); err != nil {
		_ = f.Close()
		return nil, err
	}
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		_ = f.Close()
		return nil, err
	}
	return archive, nil
}

// Close closes the underlying archive file.
func (a *DownloadArchive) Close() error {
	if a == nil || a.file == nil {
		return nil
	}
	return a.file.Close()
}

// Has reports whether videoID is already recorded.
func (a *DownloadArchive) Has(videoID string) bool {
	if a == nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := a.ids[videoID]
	return ok
}

// Add records videoID if not already present.
func (a *DownloadArchive) Add(videoID string) error {
	if a == nil {
		return nil
	}
	if _, err := ExtractVideoID(videoID); err != nil {
		return fmt.Errorf("invalid video id for archive: %q", videoID)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, exists := a.ids[videoID]; exists {
		return nil
	}
	if _, err := a.file.WriteString(videoID + "\n"); err != nil {
		return err
	}
	if err := a.file.Sync(); err != nil {
		return err
	}
	a.ids[videoID] = struct{}{}
	return nil
}
