package client

import (
	"context"
	"errors"
	"testing"
)

func TestSubtitleSidecarResultCountsAndFailures(t *testing.T) {
	errBoom := errors.New("boom")
	result := SubtitleSidecarResult{Outcomes: []SubtitleSidecarOutcome{
		{Language: "en", Path: "x.en.srt", Written: true},
		{Language: "ko", Path: "x.ko.srt", Skipped: true},
		{Language: "ja", Err: errBoom},
	}}
	if got := result.WrittenCount(); got != 1 {
		t.Fatalf("WrittenCount()=%d, want 1", got)
	}
	failures := result.FailureMessages()
	if len(failures) != 1 || failures[0] != "ja(boom)" {
		t.Fatalf("FailureMessages()=%v", failures)
	}
}

func TestWriteRequestedSubtitleSidecarsBeforeEachError(t *testing.T) {
	c := New(Config{})
	wantErr := errors.New("sleep canceled")
	_, err := c.WriteRequestedSubtitleSidecars(context.Background(), "jNQXAC9IVRw", &VideoInfo{ID: "jNQXAC9IVRw"}, SubtitleSidecarOptions{
		Languages:    []string{"en"},
		OutputFormat: SubtitleOutputFormatSRT,
		BeforeEach: func(context.Context) error {
			return wantErr
		},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("WriteRequestedSubtitleSidecars() error=%v, want %v", err, wantErr)
	}
}
