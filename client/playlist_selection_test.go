package client

import (
	"errors"
	"testing"
)

func TestSelectPlaylistItems_IndexesRangesAndSteps(t *testing.T) {
	items := []PlaylistItem{
		{VideoID: "one"},
		{VideoID: "two"},
		{VideoID: "three"},
		{VideoID: "four"},
		{VideoID: "five"},
	}

	got, err := SelectPlaylistItems(items, "1,3:5:2,-1")
	if err != nil {
		t.Fatalf("SelectPlaylistItems() error = %v", err)
	}
	want := []string{"one", "three", "five"}
	if len(got) != len(want) {
		t.Fatalf("len=%d, want %d", len(got), len(want))
	}
	for i, item := range got {
		if item.VideoID != want[i] {
			t.Fatalf("item[%d]=%q, want %q", i, item.VideoID, want[i])
		}
	}
}

func TestSelectPlaylistItems_NegativeStepOpenRange(t *testing.T) {
	items := []PlaylistItem{
		{VideoID: "one"},
		{VideoID: "two"},
		{VideoID: "three"},
	}
	got, err := SelectPlaylistItems(items, "::-1")
	if err != nil {
		t.Fatalf("SelectPlaylistItems() error = %v", err)
	}
	want := []string{"three", "two", "one"}
	for i, item := range got {
		if item.VideoID != want[i] {
			t.Fatalf("item[%d]=%q, want %q", i, item.VideoID, want[i])
		}
	}
}

func TestSelectPlaylistItems_InvalidSelector(t *testing.T) {
	_, err := SelectPlaylistItems([]PlaylistItem{{VideoID: "one"}}, "1:3:0")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("error=%v, want ErrInvalidInput", err)
	}
}
