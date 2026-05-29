package client

import (
	"context"
	"errors"
	"testing"
)

func TestRunPlaylistItemsContinuesAndSummarizesFailures(t *testing.T) {
	errFail := errors.New("fail")
	items := []PlaylistItem{{VideoID: "one"}, {VideoID: "two"}, {VideoID: "three"}}
	summary, failures := RunPlaylistItems(context.Background(), items, PlaylistRunOptions{}, func(_ context.Context, item PlaylistItem, _ int) error {
		if item.VideoID == "two" {
			return errFail
		}
		return nil
	})
	if summary.Total != 3 || summary.Succeeded != 2 || summary.Failed != 1 || summary.Aborted || summary.Skipped != 0 {
		t.Fatalf("summary=%+v", summary)
	}
	if len(failures) != 1 || failures[0].VideoID != "two" || !errors.Is(failures[0].Err, errFail) {
		t.Fatalf("failures=%+v", failures)
	}
}

func TestRunPlaylistItemsStopsAtFailureThreshold(t *testing.T) {
	items := []PlaylistItem{{VideoID: "one"}, {VideoID: "two"}, {VideoID: "three"}}
	summary, failures := RunPlaylistItems(context.Background(), items, PlaylistRunOptions{SkipAfterErrors: 1}, func(context.Context, PlaylistItem, int) error {
		return errors.New("fail")
	})
	if !summary.Aborted || summary.Failed != 1 || summary.Skipped != 2 || len(failures) != 1 {
		t.Fatalf("summary=%+v failures=%+v", summary, failures)
	}
}

func TestApplyPlaylistTemplateContext(t *testing.T) {
	got := ApplyPlaylistTemplateContext(
		"%(playlist_index)s-%(playlist_autonumber)s-%(playlist_count)s-%(playlist_title)s-%(playlist_channel_id)s.%(ext)s",
		PlaylistItem{VideoID: "v", PlaylistIndex: 7},
		2,
		PlaylistTemplateContext{Title: "A/B", ChannelID: "UC:123", Count: 9},
	)
	want := "00007-00002-00009-A_B-UC_123.%(ext)s"
	if got != want {
		t.Fatalf("ApplyPlaylistTemplateContext()=%q, want %q", got, want)
	}
}
