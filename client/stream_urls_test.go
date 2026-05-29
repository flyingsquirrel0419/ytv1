package client

import (
	"errors"
	"fmt"
	"testing"
)

func TestSelectedStreamURLsUsesDirectURLWhenAvailable(t *testing.T) {
	got, err := SelectedStreamURLs([]FormatInfo{
		{Itag: 18, URL: "https://media.example/direct.mp4", MimeType: "video/mp4", HasAudio: true, HasVideo: true},
	}, func(itag int) (string, error) {
		t.Fatalf("resolver should not be called for direct URL, got itag=%d", itag)
		return "", nil
	})
	if err != nil {
		t.Fatalf("SelectedStreamURLs() error = %v", err)
	}
	if len(got) != 1 || got[0] != "https://media.example/direct.mp4" {
		t.Fatalf("urls=%v", got)
	}
}

func TestSelectedStreamURLsResolvesCipheredFormats(t *testing.T) {
	got, err := SelectedStreamURLs([]FormatInfo{
		{Itag: 137, Ciphered: true, HasVideo: true},
		{Itag: 140, Ciphered: true, HasAudio: true},
	}, func(itag int) (string, error) {
		return fmt.Sprintf("https://media.example/%d", itag), nil
	})
	if err != nil {
		t.Fatalf("SelectedStreamURLs() error = %v", err)
	}
	want := []string{"https://media.example/137", "https://media.example/140"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("urls=%v, want %v", got, want)
		}
	}
}

func TestSelectedStreamURLsRequiresResolverForUnavailableDirectURL(t *testing.T) {
	_, err := SelectedStreamURLs([]FormatInfo{{Itag: 140, Ciphered: true}}, nil)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("SelectedStreamURLs() error = %v, want ErrUnavailable", err)
	}
}
