package client

import "testing"

func TestBuildYTDLPDumpSingleJSON(t *testing.T) {
	info := &VideoInfo{
		ID:    "jNQXAC9IVRw",
		Title: "Me at the zoo",
		Formats: []FormatInfo{
			{Itag: 140, URL: "https://cdn/audio.m4a", MimeType: "audio/mp4", HasAudio: true, Bitrate: 128000, Protocol: "https"},
			{Itag: 22, URL: "https://cdn/video.mp4", MimeType: "video/mp4", HasVideo: true, HasAudio: true, Width: 1280, Height: 720, FPS: 30, Bitrate: 2000000, Protocol: "https"},
			{Itag: 248, MimeType: "video/webm", HasVideo: true, Width: 1920, Height: 1080, Bitrate: 3000000},
		},
	}
	payload := BuildYTDLPDumpSingleJSON("https://youtu.be/jNQXAC9IVRw", info)
	if payload.ID != "jNQXAC9IVRw" || payload.ExtractorKey != "Youtube" {
		t.Fatalf("unexpected payload identity: %+v", payload)
	}
	if payload.WebpageURL != "https://www.youtube.com/watch?v=jNQXAC9IVRw" {
		t.Fatalf("WebpageURL=%q", payload.WebpageURL)
	}
	if payload.URL != "https://cdn/video.mp4" || payload.Ext != "mp4" {
		t.Fatalf("best url/ext=%q/%q", payload.URL, payload.Ext)
	}
	if len(payload.Formats) != 2 {
		t.Fatalf("formats len=%d, want 2", len(payload.Formats))
	}
}

func TestBuildYTDLPPlaylistInfoJSON(t *testing.T) {
	payload := BuildYTDLPPlaylistInfoJSON(&PlaylistInfo{
		ID:         "PL123",
		Title:      "Playlist",
		Channel:    "Channel",
		ChannelID:  "UC123",
		Uploader:   "Uploader",
		UploaderID: "UU123",
		Items: []PlaylistItem{
			{VideoID: "jNQXAC9IVRw", Title: "one", DurationSec: 19, Author: "jawed"},
		},
	})
	if payload.ID != "PL123" || payload.ExtractorKey != "YoutubePlaylist" {
		t.Fatalf("unexpected payload identity: %+v", payload)
	}
	if payload.WebpageURL != "https://www.youtube.com/playlist?list=PL123" {
		t.Fatalf("WebpageURL=%q", payload.WebpageURL)
	}
	if len(payload.Entries) != 1 || payload.Entries[0].PlaylistIndex != 1 {
		t.Fatalf("entries=%+v", payload.Entries)
	}
}

func TestPlaylistInfoAsVideoInfo(t *testing.T) {
	got := PlaylistInfoAsVideoInfo(&PlaylistInfo{
		ID:         "PL123",
		Title:      "Playlist",
		Channel:    "Channel",
		ChannelID:  "UC123",
		Uploader:   "Uploader",
		UploaderID: "UU123",
	})
	if got.ID != "PL123" || got.Title != "Playlist" || got.Author != "Uploader" || got.ChannelID != "UC123" {
		t.Fatalf("PlaylistInfoAsVideoInfo()=%+v", got)
	}
}
