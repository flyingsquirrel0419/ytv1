package client

import (
	"fmt"
	"strings"
)

// MetadataPrintData contains values used by yt-dlp-style metadata print fields.
type MetadataPrintData struct {
	Info          *VideoInfo
	Input         string
	Filename      string
	FormatSummary string
	URL           string
	ThumbnailURL  string

	OutputTemplate OutputTemplateData

	Description string
	Duration    string
	UploadDate  string
	ReleaseDate string
	Timestamp   string
}

// RenderMetadataPrintTemplate renders a metadata field or template expression.
func RenderMetadataPrintTemplate(template string, data MetadataPrintData) (string, error) {
	template = strings.TrimSpace(StripPrintStagePrefix(template))
	info := data.Info
	if info == nil {
		return "", fmt.Errorf("%w: missing video info", ErrInvalidInput)
	}
	if data.Description == "" {
		data.Description = info.Description
	}
	if data.Duration == "" {
		data.Duration = FormatDuration(info.DurationSec)
	}
	if data.ThumbnailURL == "" {
		data.ThumbnailURL = info.ThumbnailURL
	}
	if data.OutputTemplate.UploadDate == "" {
		data.OutputTemplate.UploadDate = data.UploadDate
	}
	if data.OutputTemplate.ReleaseDate == "" {
		data.OutputTemplate.ReleaseDate = data.ReleaseDate
	}
	if data.OutputTemplate.Timestamp == "" {
		data.OutputTemplate.Timestamp = data.Timestamp
	}
	switch template {
	case "title":
		return info.Title, nil
	case "id":
		return info.ID, nil
	case "description":
		return data.Description, nil
	case "duration":
		return data.Duration, nil
	case "filename":
		return data.Filename, nil
	case "format":
		return data.FormatSummary, nil
	case "url":
		return data.URL, nil
	case "thumbnail", "thumbnail_url":
		if strings.TrimSpace(data.ThumbnailURL) == "" {
			return "", fmt.Errorf("%w: thumbnail unavailable for video=%s", ErrUnavailable, info.ID)
		}
		return data.ThumbnailURL, nil
	case "webpage_url", "original_url":
		return data.Input, nil
	}

	rendered := RenderOutputTemplate(template, data.OutputTemplate)
	replacements := map[string]string{
		"%(description)s":   data.Description,
		"%(duration)s":      data.Duration,
		"%(filename)s":      data.Filename,
		"%(format)s":        data.FormatSummary,
		"%(url)s":           data.URL,
		"%(thumbnail)s":     data.ThumbnailURL,
		"%(thumbnail_url)s": data.ThumbnailURL,
		"%(webpage_url)s":   data.Input,
		"%(original_url)s":  data.Input,
		"%(upload_date)s":   data.UploadDate,
		"%(release_date)s":  data.ReleaseDate,
		"%(timestamp)s":     data.Timestamp,
	}
	for token, value := range replacements {
		rendered = strings.ReplaceAll(rendered, token, value)
	}
	return rendered, nil
}

// StripPrintStagePrefix removes yt-dlp print stage prefixes accepted by ytv1.
func StripPrintStagePrefix(template string) string {
	for _, prefix := range []string{"video:", "before_dl:", "after_move:"} {
		if strings.HasPrefix(template, prefix) {
			return strings.TrimPrefix(template, prefix)
		}
	}
	return template
}

// FormatDuration renders seconds as M:SS or H:MM:SS.
func FormatDuration(seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}
