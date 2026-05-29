package client

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteDescriptionSidecar writes the video's description to outputPath.
func WriteDescriptionSidecar(info *VideoInfo, outputPath string) error {
	outputPath = strings.TrimSpace(outputPath)
	if outputPath == "" {
		return fmt.Errorf("%w: empty description output path", ErrInvalidInput)
	}
	description := ""
	if info != nil {
		description = info.Description
	}
	dir := filepath.Dir(outputPath)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("failed to create description directory: %w", err)
		}
	}
	if err := os.WriteFile(outputPath, []byte(description), 0o644); err != nil {
		return fmt.Errorf("failed to write description: %w", err)
	}
	return nil
}
