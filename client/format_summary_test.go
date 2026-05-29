package client

import "testing"

func TestFormatMediaExt(t *testing.T) {
	tests := []struct {
		name     string
		mimeType string
		want     string
	}{
		{name: "plain subtype", mimeType: "video/mp4", want: "mp4"},
		{name: "with parameters", mimeType: `audio/mp4; codecs="mp4a.40.2"`, want: "mp4"},
		{name: "invalid", mimeType: "not a mime", want: "?"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatMediaExt(tt.mimeType); got != tt.want {
				t.Fatalf("FormatMediaExt(%q) = %q, want %q", tt.mimeType, got, tt.want)
			}
		})
	}
}

func TestFormatTrackNote(t *testing.T) {
	tests := []struct {
		name   string
		format FormatInfo
		want   string
	}{
		{name: "audio only", format: FormatInfo{HasAudio: true}, want: "audio only"},
		{name: "video only", format: FormatInfo{HasVideo: true}, want: "video only"},
		{name: "av", format: FormatInfo{HasAudio: true, HasVideo: true}, want: "av"},
		{name: "unknown", format: FormatInfo{}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatTrackNote(tt.format); got != tt.want {
				t.Fatalf("FormatTrackNote() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatSummaries(t *testing.T) {
	formats := []FormatInfo{
		{Itag: 137, MimeType: "video/mp4", HasVideo: true, Width: 1920, Height: 1080},
		{Itag: 140, MimeType: "audio/mp4", HasAudio: true},
	}

	got := FormatSummaries(formats)
	want := "137 - mp4 1920x1080 video only + 140 - mp4 audio only audio only"
	if got != want {
		t.Fatalf("FormatSummaries() = %q, want %q", got, want)
	}
}
