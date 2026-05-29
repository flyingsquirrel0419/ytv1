package client

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSidecarOutputPaths(t *testing.T) {
	info := &VideoInfo{
		ID:           "abc123",
		Title:        "Title/Bad",
		Author:       "Uploader",
		ChannelID:    "UC123",
		ThumbnailURL: "https://i.example.com/thumb.webp?x=1",
	}
	template := filepath.Join("out", "%(title)s.%(ext)s")
	opts := SidecarPathOptions{OutputTemplate: template}

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"subtitle", SubtitleOutputPath(info, "EN", "vtt", opts), filepath.Join("out", "Title_Bad.en.vtt")},
		{"info", InfoJSONOutputPath(info, opts), filepath.Join("out", "Title_Bad.info.json")},
		{"description", DescriptionOutputPath(info, opts), filepath.Join("out", "Title_Bad.description")},
		{"shortcut", ShortcutOutputPath(info, ShortcutURL, opts), filepath.Join("out", "Title_Bad.url")},
		{"thumbnail", ThumbnailOutputPath(info, opts), filepath.Join("out", "Title_Bad.webp")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("path=%q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestSidecarOutputPaths_RestrictedAndTrimmed(t *testing.T) {
	info := &VideoInfo{ID: "abc123", Title: "A title 한글 / symbol"}
	got := DescriptionOutputPath(info, SidecarPathOptions{
		OutputTemplate:    "%(title)s.%(ext)s",
		RestrictFilenames: true,
		TrimFilenames:     7,
	})
	if got != "A_title.description" {
		t.Fatalf("DescriptionOutputPath()=%q, want A_title.description", got)
	}
}

func TestThumbnailExt(t *testing.T) {
	if got := ThumbnailExt("https://example.com/image.jpeg?x=1"); got != "jpeg" {
		t.Fatalf("ThumbnailExt()=%q, want jpeg", got)
	}
	if got := ThumbnailExt("https://example.com/image.gif"); got != "jpg" {
		t.Fatalf("ThumbnailExt()=%q, want jpg", got)
	}
}

func TestShortcutSidecarBody(t *testing.T) {
	if got := ShortcutSidecarBody("https://youtu.be/jNQXAC9IVRw", nil, ShortcutURL); got != "[InternetShortcut]\r\nURL=https://youtu.be/jNQXAC9IVRw\r\n" {
		t.Fatalf("url shortcut=%q", got)
	}
	webloc := ShortcutSidecarBody("https://youtu.be/watch?v=1&x=2", nil, ShortcutWebloc)
	if !strings.Contains(webloc, "https://youtu.be/watch?v=1&amp;x=2") {
		t.Fatalf("webloc body did not escape URL: %q", webloc)
	}
	desktop := ShortcutSidecarBody("https://youtu.be/a\nb", &VideoInfo{Title: "Line\nTitle"}, ShortcutDesktop)
	if !strings.Contains(desktop, "Name=Line\\nTitle") || !strings.Contains(desktop, "URL=https://youtu.be/a\\nb") {
		t.Fatalf("desktop body did not escape fields: %q", desktop)
	}
}
