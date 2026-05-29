package client

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteShortcutSidecarURL(t *testing.T) {
	out := filepath.Join(t.TempDir(), "nested", "video.url")
	err := WriteShortcutSidecar("https://youtu.be/jNQXAC9IVRw", &VideoInfo{Title: "Me at the zoo"}, ShortcutURL, out)
	if err != nil {
		t.Fatalf("WriteShortcutSidecar() error = %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	want := "[InternetShortcut]\r\nURL=https://youtu.be/jNQXAC9IVRw\r\n"
	if string(got) != want {
		t.Fatalf("url sidecar=%q, want %q", got, want)
	}
}

func TestWriteShortcutSidecarWebloc(t *testing.T) {
	out := filepath.Join(t.TempDir(), "video.webloc")
	err := WriteShortcutSidecar("https://youtu.be/watch?v=1&x=2", nil, ShortcutWebloc, out)
	if err != nil {
		t.Fatalf("WriteShortcutSidecar() error = %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(got), "<key>URL</key>") || !strings.Contains(string(got), "watch?v=1&amp;x=2") {
		t.Fatalf("unexpected webloc sidecar: %q", got)
	}
}

func TestWriteShortcutSidecarEmptyPath(t *testing.T) {
	err := WriteShortcutSidecar("https://youtu.be/jNQXAC9IVRw", nil, ShortcutURL, "")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("error=%v, want ErrInvalidInput", err)
	}
}
