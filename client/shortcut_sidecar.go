package client

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteShortcutSidecar writes an internet shortcut sidecar to outputPath.
func WriteShortcutSidecar(input string, info *VideoInfo, kind ShortcutKind, outputPath string) error {
	outputPath = strings.TrimSpace(outputPath)
	if outputPath == "" {
		return fmt.Errorf("%w: empty shortcut output path", ErrInvalidInput)
	}
	dir := filepath.Dir(outputPath)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("failed to create shortcut directory: %w", err)
		}
	}
	body := ShortcutSidecarBody(input, info, kind)
	if err := os.WriteFile(outputPath, []byte(body), 0o644); err != nil {
		return fmt.Errorf("failed to write %s link: %w", kind, err)
	}
	return nil
}
