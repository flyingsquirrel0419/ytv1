package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// DownloadThumbnail downloads the selected thumbnail for info to outputPath.
func (c *Client) DownloadThumbnail(ctx context.Context, info *VideoInfo, outputPath string) error {
	if info == nil || strings.TrimSpace(info.ThumbnailURL) == "" {
		videoID := ""
		if info != nil {
			videoID = info.ID
		}
		return fmt.Errorf("%w: thumbnail unavailable for video=%s", ErrUnavailable, videoID)
	}
	outputPath = strings.TrimSpace(outputPath)
	if outputPath == "" {
		return fmt.Errorf("%w: empty thumbnail output path", ErrInvalidInput)
	}
	dir := filepath.Dir(outputPath)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("failed to create thumbnail directory: %w", err)
		}
	}

	httpClient := http.DefaultClient
	if c != nil && c.HTTPClient() != nil {
		httpClient = c.HTTPClient()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, info.ThumbnailURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create thumbnail request: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download thumbnail: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("failed to download thumbnail: http status %d", resp.StatusCode)
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create thumbnail file: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("failed to write thumbnail: %w", err)
	}
	return nil
}
