package client

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// SidecarPathOptions controls sidecar output path rendering.
type SidecarPathOptions struct {
	OutputTemplate    string
	RestrictFilenames bool
	TrimFilenames     int
}

// ShortcutKind identifies an internet shortcut sidecar type.
type ShortcutKind string

const (
	ShortcutURL     ShortcutKind = "url"
	ShortcutWebloc  ShortcutKind = "webloc"
	ShortcutDesktop ShortcutKind = "desktop"
)

// SubtitleOutputPath returns a subtitle sidecar path.
func SubtitleOutputPath(info *VideoInfo, lang string, outputExt string, opts SidecarPathOptions) string {
	outputExt = strings.TrimSpace(strings.ToLower(outputExt))
	if outputExt == "" {
		outputExt = string(SubtitleOutputFormatSRT)
	}
	safeLang := strings.TrimSpace(strings.ToLower(lang))
	if safeLang == "" {
		safeLang = "unknown"
	}
	if strings.TrimSpace(opts.OutputTemplate) == "" {
		return TrimOutputPathFilename(fmt.Sprintf("%s.%s.%s", safeVideoID(info), safeLang, outputExt), opts.TrimFilenames)
	}
	base := renderSidecarBase(opts.OutputTemplate, info, outputExt, "subs_"+safeLang, opts.RestrictFilenames)
	if strings.TrimSpace(base) == "" {
		return TrimOutputPathFilename(fmt.Sprintf("%s.%s.%s", safeVideoID(info), safeLang, outputExt), opts.TrimFilenames)
	}
	if ext := filepath.Ext(base); ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	return TrimOutputPathFilename(base+"."+safeLang+"."+outputExt, opts.TrimFilenames)
}

// InfoJSONOutputPath returns a video or playlist info JSON sidecar path.
func InfoJSONOutputPath(info *VideoInfo, opts SidecarPathOptions) string {
	if strings.TrimSpace(opts.OutputTemplate) == "" {
		return TrimOutputPathFilename(SanitizeOutputTemplateToken(safeVideoID(info))+".info.json", opts.TrimFilenames)
	}
	base := renderSidecarBase(opts.OutputTemplate, info, "info", "info", opts.RestrictFilenames)
	if strings.TrimSpace(base) == "" {
		return TrimOutputPathFilename(SanitizeOutputTemplateToken(safeVideoID(info))+".info.json", opts.TrimFilenames)
	}
	if ext := filepath.Ext(base); ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	return TrimOutputPathFilename(base+".info.json", opts.TrimFilenames)
}

// DescriptionOutputPath returns a description sidecar path.
func DescriptionOutputPath(info *VideoInfo, opts SidecarPathOptions) string {
	if strings.TrimSpace(opts.OutputTemplate) == "" {
		return TrimOutputPathFilename(SanitizeOutputTemplateToken(safeVideoID(info))+".description", opts.TrimFilenames)
	}
	base := renderSidecarBase(opts.OutputTemplate, info, "description", "description", opts.RestrictFilenames)
	if strings.TrimSpace(base) == "" {
		return TrimOutputPathFilename(SanitizeOutputTemplateToken(safeVideoID(info))+".description", opts.TrimFilenames)
	}
	if ext := filepath.Ext(base); ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	return TrimOutputPathFilename(base+".description", opts.TrimFilenames)
}

// ShortcutOutputPath returns an internet shortcut sidecar path.
func ShortcutOutputPath(info *VideoInfo, kind ShortcutKind, opts SidecarPathOptions) string {
	ext := string(kind)
	if ext == "" {
		ext = string(ShortcutURL)
	}
	if strings.TrimSpace(opts.OutputTemplate) == "" {
		return TrimOutputPathFilename(SanitizeOutputTemplateToken(safeVideoID(info))+"."+ext, opts.TrimFilenames)
	}
	base := renderSidecarBase(opts.OutputTemplate, info, ext, ext, opts.RestrictFilenames)
	if strings.TrimSpace(base) == "" {
		return TrimOutputPathFilename(SanitizeOutputTemplateToken(safeVideoID(info))+"."+ext, opts.TrimFilenames)
	}
	if pathExt := filepath.Ext(base); pathExt != "" {
		base = strings.TrimSuffix(base, pathExt)
	}
	return TrimOutputPathFilename(base+"."+ext, opts.TrimFilenames)
}

// ThumbnailOutputPath returns a thumbnail sidecar path.
func ThumbnailOutputPath(info *VideoInfo, opts SidecarPathOptions) string {
	ext := ThumbnailExt("")
	if info != nil {
		ext = ThumbnailExt(info.ThumbnailURL)
	}
	if strings.TrimSpace(opts.OutputTemplate) == "" {
		return TrimOutputPathFilename(SanitizeOutputTemplateToken(safeVideoID(info))+"."+ext, opts.TrimFilenames)
	}
	base := renderSidecarBase(opts.OutputTemplate, info, ext, "thumbnail", opts.RestrictFilenames)
	if strings.TrimSpace(base) == "" {
		return TrimOutputPathFilename(SanitizeOutputTemplateToken(safeVideoID(info))+"."+ext, opts.TrimFilenames)
	}
	if pathExt := filepath.Ext(base); pathExt != "" {
		base = strings.TrimSuffix(base, pathExt)
	}
	return TrimOutputPathFilename(base+"."+ext, opts.TrimFilenames)
}

// ThumbnailExt returns a supported thumbnail file extension.
func ThumbnailExt(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	path := strings.TrimSpace(rawURL)
	if err == nil && u.Path != "" {
		path = u.Path
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	switch ext {
	case "jpg", "jpeg", "png", "webp":
		return ext
	default:
		return "jpg"
	}
}

// ShortcutSidecarBody renders the contents of an internet shortcut sidecar.
func ShortcutSidecarBody(input string, info *VideoInfo, kind ShortcutKind) string {
	url := strings.TrimSpace(input)
	switch kind {
	case ShortcutWebloc:
		return `<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
			`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n" +
			`<plist version="1.0">` + "\n" +
			"<dict>\n" +
			"\t<key>URL</key>\n" +
			"\t<string>" + xmlEscape(url) + "</string>\n" +
			"</dict>\n" +
			"</plist>\n"
	case ShortcutDesktop:
		name := "YouTube"
		if info != nil && strings.TrimSpace(info.Title) != "" {
			name = strings.TrimSpace(info.Title)
		}
		return "[Desktop Entry]\n" +
			"Type=Link\n" +
			"Name=" + desktopEntryEscape(name) + "\n" +
			"URL=" + desktopEntryEscape(url) + "\n"
	default:
		return "[InternetShortcut]\r\nURL=" + url + "\r\n"
	}
}

func xmlEscape(v string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(v)
}

func desktopEntryEscape(v string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"\n", `\n`,
		"\r", "",
	)
	return replacer.Replace(v)
}

func renderSidecarBase(template string, info *VideoInfo, ext string, formatID string, restrict bool) string {
	if info == nil {
		info = &VideoInfo{}
	}
	return RenderOutputTemplate(template, OutputTemplateData{
		VideoID:           info.ID,
		Title:             info.Title,
		Uploader:          info.Author,
		UploaderID:        info.ChannelID,
		Channel:           info.Author,
		ChannelID:         info.ChannelID,
		Ext:               ext,
		Itag:              formatID,
		FormatID:          formatID,
		UploadDate:        templateUploadDate(info),
		ReleaseDate:       templateReleaseDate(info),
		Timestamp:         templateTimestamp(info),
		RestrictFilenames: restrict,
	})
}

func safeVideoID(info *VideoInfo) string {
	if info == nil || strings.TrimSpace(info.ID) == "" {
		return "unknown"
	}
	return info.ID
}
