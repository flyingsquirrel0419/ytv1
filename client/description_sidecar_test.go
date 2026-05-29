package client

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteDescriptionSidecar(t *testing.T) {
	out := filepath.Join(t.TempDir(), "nested", "video.description")
	err := WriteDescriptionSidecar(&VideoInfo{Description: "line 1\nline 2"}, out)
	if err != nil {
		t.Fatalf("WriteDescriptionSidecar() error = %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "line 1\nline 2" {
		t.Fatalf("description=%q, want line 1\\nline 2", got)
	}
}

func TestWriteDescriptionSidecarEmptyPath(t *testing.T) {
	err := WriteDescriptionSidecar(&VideoInfo{Description: "text"}, "")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("error=%v, want ErrInvalidInput", err)
	}
}
