package client

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// MetadataPrintFileSpec describes one --print-to-file request.
type MetadataPrintFileSpec struct {
	Template string
	File     string
}

// MetadataPrintRequest controls package-level metadata/get/print rendering.
type MetadataPrintRequest struct {
	Order             []string
	PrintFields       []string
	PrintToFile       []MetadataPrintFileSpec
	DownloadOpts      DownloadOptions
	FilenameOpts      OutputFilenameOptions
	RestrictFilenames bool
}

// MetadataPrintItem is one rendered metadata output action.
type MetadataPrintItem struct {
	Value string
	File  string
}

// RenderMetadataPrintItems renders metadata outputs in the requested order.
func RenderMetadataPrintItems(info *VideoInfo, input string, req MetadataPrintRequest, resolveURL StreamURLResolver) ([]MetadataPrintItem, error) {
	order := append([]string(nil), req.Order...)
	for _, template := range req.PrintFields {
		order = append(order, "print:"+template)
	}
	for _, spec := range req.PrintToFile {
		order = append(order, "printfile:"+spec.Template+"\x00"+spec.File)
	}

	items := make([]MetadataPrintItem, 0, len(order))
	for _, field := range order {
		if strings.HasPrefix(field, "printfile:") {
			spec, ok := ParseMetadataPrintFileOrderField(strings.TrimPrefix(field, "printfile:"))
			if !ok {
				continue
			}
			line, err := RenderMetadataPrintField(strings.TrimSpace(spec.Template), info, input, req, resolveURL)
			if err != nil {
				return nil, err
			}
			path, err := MetadataPrintFilePath(spec.File, info, req.RestrictFilenames)
			if err != nil {
				return nil, err
			}
			items = append(items, MetadataPrintItem{Value: line, File: path})
			continue
		}
		if strings.HasPrefix(field, "print:") {
			line, err := RenderMetadataPrintField(strings.TrimPrefix(field, "print:"), info, input, req, resolveURL)
			if err != nil {
				return nil, err
			}
			items = append(items, MetadataPrintItem{Value: line})
			continue
		}
		line, err := RenderMetadataPrintField(field, info, input, req, resolveURL)
		if err != nil {
			return nil, err
		}
		items = append(items, MetadataPrintItem{Value: line})
	}
	return items, nil
}

// RenderMetadataPrintField renders one metadata field or template.
func RenderMetadataPrintField(template string, info *VideoInfo, input string, req MetadataPrintRequest, resolveURL StreamURLResolver) (string, error) {
	template = strings.TrimSpace(StripPrintStagePrefix(template))
	filename, filenameErr := metadataFilename(info, req)
	if filenameErr != nil && (template == "filename" || strings.Contains(template, "%(filename)s")) {
		return "", filenameErr
	}
	formatSummary := ""
	if template == "format" || strings.Contains(template, "%(format)s") {
		formats, err := SelectFormatsForDownloadOptions(info.Formats, req.DownloadOpts)
		if err != nil {
			return "", err
		}
		formatSummary = FormatSummaries(formats)
	}
	streamURL := ""
	if template == "url" || strings.Contains(template, "%(url)s") {
		urls, err := metadataSelectedStreamURLs(info, req, resolveURL)
		if err != nil {
			return "", err
		}
		streamURL = strings.Join(urls, "\n")
	}
	switch template {
	case "filename":
		return filename, nil
	case "format":
		return formatSummary, nil
	case "url":
		return streamURL, nil
	}

	outputTemplate, err := metadataOutputTemplateData(info, req)
	if err != nil {
		outputTemplate = OutputTemplateData{}
	}
	return RenderMetadataPrintTemplate(template, MetadataPrintData{
		Info:           info,
		Input:          input,
		Filename:       filename,
		FormatSummary:  formatSummary,
		URL:            streamURL,
		ThumbnailURL:   info.ThumbnailURL,
		Description:    info.Description,
		Duration:       FormatDuration(info.DurationSec),
		UploadDate:     templateUploadDate(info),
		ReleaseDate:    templateReleaseDate(info),
		Timestamp:      templateTimestamp(info),
		OutputTemplate: outputTemplate,
	})
}

// ParseMetadataPrintFileOrderField parses the internal printfile order form.
func ParseMetadataPrintFileOrderField(raw string) (MetadataPrintFileSpec, bool) {
	parts := strings.SplitN(raw, "\x00", 2)
	if len(parts) != 2 {
		return MetadataPrintFileSpec{}, false
	}
	return MetadataPrintFileSpec{Template: parts[0], File: parts[1]}, true
}

// MetadataPrintFilePath renders a print-to-file path template.
func MetadataPrintFilePath(rawPath string, info *VideoInfo, restrictFilenames bool) (string, error) {
	path := strings.TrimSpace(rawPath)
	if path == "" {
		return "", fmt.Errorf("%w: empty --print-to-file path", ErrInvalidInput)
	}
	return RenderOutputTemplate(path, OutputTemplateData{
		VideoID:           info.ID,
		Title:             info.Title,
		Uploader:          info.Author,
		UploaderID:        info.ChannelID,
		Channel:           info.Author,
		ChannelID:         info.ChannelID,
		Ext:               "txt",
		Itag:              "print",
		FormatID:          "print",
		UploadDate:        templateUploadDate(info),
		ReleaseDate:       templateReleaseDate(info),
		Timestamp:         templateTimestamp(info),
		RestrictFilenames: restrictFilenames,
	}), nil
}

// AppendMetadataPrintFile appends one rendered print line to a file.
func AppendMetadataPrintFile(path string, line string) error {
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create print-to-file directory: %w", err)
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open print-to-file output: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(line + "\n"); err != nil {
		return fmt.Errorf("failed to write print-to-file output: %w", err)
	}
	return nil
}

func metadataFilename(info *VideoInfo, req MetadataPrintRequest) (string, error) {
	formats, err := SelectFormatsForDownloadOptions(info.Formats, req.DownloadOpts)
	if err != nil {
		return "", err
	}
	return PredictOutputFilename(info, formats, req.FilenameOpts)
}

func metadataSelectedStreamURLs(info *VideoInfo, req MetadataPrintRequest, resolveURL StreamURLResolver) ([]string, error) {
	formats, err := SelectFormatsForDownloadOptions(info.Formats, req.DownloadOpts)
	if err != nil {
		return nil, err
	}
	return SelectedStreamURLs(formats, resolveURL)
}

func metadataOutputTemplateData(info *VideoInfo, req MetadataPrintRequest) (OutputTemplateData, error) {
	formats, err := SelectFormatsForDownloadOptions(info.Formats, req.DownloadOpts)
	if err != nil {
		return OutputTemplateData{}, err
	}
	ext := ""
	itag := ""
	if len(formats) > 0 {
		if IsMergedSelection(formats) {
			videoItag, audioItag := MergedItags(formats)
			ext = NormalizeMergeOutputExt(req.FilenameOpts.MergeOutputExt)
			itag = fmt.Sprintf("%d+%d", videoItag, audioItag)
		} else {
			ext = DetectOutputExt(formats[0].MimeType, req.DownloadOpts.Mode)
			itag = strconv.Itoa(formats[0].Itag)
		}
	}
	formatTokens := SelectedFormatTemplateTokens(formats)
	return OutputTemplateData{
		VideoID:           info.ID,
		Title:             info.Title,
		Uploader:          info.Author,
		UploaderID:        info.ChannelID,
		Channel:           info.Author,
		ChannelID:         info.ChannelID,
		Ext:               ext,
		Itag:              itag,
		FormatID:          itag,
		Resolution:        formatTokens.Resolution,
		Width:             formatTokens.Width,
		Height:            formatTokens.Height,
		FPS:               formatTokens.FPS,
		TBR:               formatTokens.TBR,
		VBR:               formatTokens.VBR,
		ABR:               formatTokens.ABR,
		Protocol:          formatTokens.Protocol,
		VCodec:            formatTokens.VCodec,
		ACodec:            formatTokens.ACodec,
		UploadDate:        templateUploadDate(info),
		ReleaseDate:       templateReleaseDate(info),
		Timestamp:         templateTimestamp(info),
		RestrictFilenames: req.RestrictFilenames,
	}, nil
}
