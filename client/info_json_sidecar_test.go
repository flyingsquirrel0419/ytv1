package client

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteInfoJSONSidecar(t *testing.T) {
	out := filepath.Join(t.TempDir(), "nested", "video.info.json")
	err := WriteInfoJSONSidecar("https://youtu.be/jNQXAC9IVRw", &VideoInfo{
		ID:    "jNQXAC9IVRw",
		Title: "Me at the zoo",
		Formats: []FormatInfo{
			{Itag: 18, URL: "https://cdn/video.mp4", MimeType: "video/mp4", HasVideo: true, HasAudio: true},
		},
	}, out)
	if err != nil {
		t.Fatalf("WriteInfoJSONSidecar() error = %v", err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var payload YTDLPDumpSingleJSON
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.ID != "jNQXAC9IVRw" || payload.URL != "https://cdn/video.mp4" {
		t.Fatalf("payload=%+v", payload)
	}
}

func TestWritePlaylistInfoJSONSidecar(t *testing.T) {
	out := filepath.Join(t.TempDir(), "playlist.info.json")
	err := WritePlaylistInfoJSONSidecar(&PlaylistInfo{
		ID:    "PL123",
		Title: "Playlist",
		Items: []PlaylistItem{{VideoID: "jNQXAC9IVRw", Title: "one"}},
	}, out)
	if err != nil {
		t.Fatalf("WritePlaylistInfoJSONSidecar() error = %v", err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var payload YTDLPPlaylistInfoJSON
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.ID != "PL123" || len(payload.Entries) != 1 {
		t.Fatalf("payload=%+v", payload)
	}
}
