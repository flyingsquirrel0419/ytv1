package client

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteInfoJSONSidecar writes a yt-dlp-style single-video .info.json sidecar.
func WriteInfoJSONSidecar(input string, info *VideoInfo, outputPath string) error {
	if err := mkdirParent(outputPath, "info json"); err != nil {
		return err
	}
	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create info json: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(BuildYTDLPDumpSingleJSON(input, info)); err != nil {
		return fmt.Errorf("failed to write info json: %w", err)
	}
	return nil
}

// WritePlaylistInfoJSONSidecar writes a yt-dlp-style playlist .info.json sidecar.
func WritePlaylistInfoJSONSidecar(playlist *PlaylistInfo, outputPath string) error {
	if err := mkdirParent(outputPath, "playlist info json"); err != nil {
		return err
	}
	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create playlist info json: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(BuildYTDLPPlaylistInfoJSON(playlist)); err != nil {
		return fmt.Errorf("failed to write playlist info json: %w", err)
	}
	return nil
}

func mkdirParent(outputPath string, label string) error {
	dir := filepath.Dir(outputPath)
	if dir == "." || dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create %s directory: %w", label, err)
	}
	return nil
}
