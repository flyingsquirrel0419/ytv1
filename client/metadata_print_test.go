package client

import (
	"errors"
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
