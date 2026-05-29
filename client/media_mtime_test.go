package client

import "testing"

func TestMediaFileMTime(t *testing.T) {
	got, ok := MediaFileMTime(&VideoInfo{UploadDate: "2005-04-23"})
	if !ok {
		t.Fatalf("MediaFileMTime() ok=false, want true")
	}
	if got.Format("2006-01-02") != "2005-04-23" {
		t.Fatalf("mtime=%s, want 2005-04-23", got.Format("2006-01-02"))
	}

	got, ok = MediaFileMTime(&VideoInfo{PublishDate: "20050424"})
	if !ok {
		t.Fatalf("MediaFileMTime compact ok=false, want true")
	}
	if got.Format("2006-01-02") != "2005-04-24" {
		t.Fatalf("mtime=%s, want 2005-04-24", got.Format("2006-01-02"))
	}

	if _, ok := MediaFileMTime(&VideoInfo{UploadDate: "not-a-date"}); ok {
		t.Fatalf("MediaFileMTime invalid ok=true, want false")
	}
	if _, ok := MediaFileMTime(nil); ok {
		t.Fatalf("MediaFileMTime(nil) ok=true, want false")
	}
}
