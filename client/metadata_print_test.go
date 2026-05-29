package client

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFormatDuration(t *testing.T) {
	tests := map[int64]string{
		-1:   "0:00",
		0:    "0:00",
		65:   "1:05",
		3661: "1:01:01",
	}
	for in, want := range tests {
		if got := FormatDuration(in); got != want {
			t.Fatalf("FormatDuration(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestRenderMetadataPrintTemplate_FieldAndTokens(t *testing.T) {
	data := MetadataPrintData{
		Info: &VideoInfo{
			ID:           "abc123",
			Title:        "Example",
			Description:  "long text",
			DurationSec:  65,
			ThumbnailURL: "https://i.ytimg.com/vi/abc123/hqdefault.jpg",
		},
		Input:         "https://youtu.be/abc123",
		Filename:      "Example.mp4",
		FormatSummary: "137 - mp4 1920x1080 video only + 140 - mp4 audio only audio only",
		URL:           "https://example.test/videoplayback",
		OutputTemplate: OutputTemplateData{
			VideoID: "abc123",
			Title:   "Example",
			Ext:     "mp4",
			Itag:    "137+140",
		},
		UploadDate: "20260529",
	}

	got, err := RenderMetadataPrintTemplate("video:%(title)s|%(id)s|%(duration)s|%(filename)s|%(upload_date)s", data)
	if err != nil {
		t.Fatalf("RenderMetadataPrintTemplate() error = %v", err)
	}
	want := "Example|abc123|1:05|Example.mp4|20260529"
	if got != want {
		t.Fatalf("RenderMetadataPrintTemplate() = %q, want %q", got, want)
	}

	got, err = RenderMetadataPrintTemplate("thumbnail_url", data)
	if err != nil {
		t.Fatalf("RenderMetadataPrintTemplate(thumbnail_url) error = %v", err)
	}
	if got != data.Info.ThumbnailURL {
		t.Fatalf("thumbnail_url = %q, want %q", got, data.Info.ThumbnailURL)
	}
}

func TestRenderMetadataPrintTemplate_ThumbnailUnavailable(t *testing.T) {
	_, err := RenderMetadataPrintTemplate("thumbnail", MetadataPrintData{Info: &VideoInfo{ID: "abc123"}})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}

func TestRenderMetadataPrintItems(t *testing.T) {
	info := &VideoInfo{
		ID:          "jNQXAC9IVRw",
		Title:       "A/B",
		Author:      "Uploader",
		ChannelID:   "UC:123",
		Description: "Description",
		DurationSec: 75,
		UploadDate:  "2005-04-23",
		PublishDate: "2005-04-24",
		Formats: []FormatInfo{
			{Itag: 18, URL: "https://media.example/18.mp4", MimeType: `video/mp4; codecs="avc1.42001E, mp4a.40.2"`, Protocol: "https", HasAudio: true, HasVideo: true, Width: 640, Height: 360, FPS: 30, Bitrate: 800000},
		},
	}
	req := MetadataPrintRequest{
		Order: []string{"title", "print:%(id)s:%(title)s:%(format_id)s:%(protocol)s:%(duration)s:%(upload_date)s:%(release_date)s:%(timestamp)s", "url"},
		DownloadOpts: DownloadOptions{
			Itag: 18,
			Mode: SelectionModeBest,
		},
		FilenameOpts:      OutputFilenameOptions{Mode: SelectionModeBest, MergeOutputExt: "mp4"},
		RestrictFilenames: false,
	}
	items, err := RenderMetadataPrintItems(info, "https://www.youtube.com/watch?v=jNQXAC9IVRw", req, nil)
	if err != nil {
		t.Fatalf("RenderMetadataPrintItems() error = %v", err)
	}
	want := []string{
		"A/B",
		"jNQXAC9IVRw:A_B:18:https:1:15:20050423:20050424:20050423000000",
		"https://media.example/18.mp4",
	}
	for i := range want {
		if items[i].Value != want[i] {
			t.Fatalf("items[%d]=%q, want %q", i, items[i].Value, want[i])
		}
	}
}

func TestAppendMetadataPrintFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "out.txt")
	if err := AppendMetadataPrintFile(path, "one"); err != nil {
		t.Fatalf("AppendMetadataPrintFile() error = %v", err)
	}
	if err := AppendMetadataPrintFile(path, "two"); err != nil {
		t.Fatalf("AppendMetadataPrintFile() second error = %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(raw) != "one\ntwo\n" {
		t.Fatalf("file=%q", string(raw))
	}
}
