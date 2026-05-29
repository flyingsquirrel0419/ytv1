package client

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownloadArchive_LoadAddAndDeduplicate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archive.txt")
	if err := os.WriteFile(path, []byte("bad line\njNQXAC9IVRw\n\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	archive, err := OpenDownloadArchive(path)
	if err != nil {
		t.Fatalf("OpenDownloadArchive() error = %v", err)
	}
	defer archive.Close()

	if !archive.Has("jNQXAC9IVRw") {
		t.Fatalf("archive should contain preloaded id")
	}
	if archive.Has("DSYFmhjDbvs") {
		t.Fatalf("archive should not contain missing id")
	}
	if err := archive.Add("DSYFmhjDbvs"); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if err := archive.Add("DSYFmhjDbvs"); err != nil {
		t.Fatalf("duplicate Add() error = %v", err)
	}
	if !archive.Has("DSYFmhjDbvs") {
		t.Fatalf("archive should contain added id")
	}
	if err := archive.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got := strings.Count(string(body), "DSYFmhjDbvs"); got != 1 {
		t.Fatalf("added id count=%d, want 1\n%s", got, string(body))
	}
}

func TestDownloadArchive_InvalidID(t *testing.T) {
	archive, err := OpenDownloadArchive(filepath.Join(t.TempDir(), "archive.txt"))
	if err != nil {
		t.Fatalf("OpenDownloadArchive() error = %v", err)
	}
	defer archive.Close()
	if err := archive.Add("not a video id"); err == nil {
		t.Fatalf("Add() error = nil, want invalid id error")
	}
}
