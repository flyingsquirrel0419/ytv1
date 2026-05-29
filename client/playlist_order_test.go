package client

import (
	"math/rand"
	"testing"
)

func TestOrderPlaylistItems_ReverseDoesNotMutate(t *testing.T) {
	items := []PlaylistItem{{VideoID: "one"}, {VideoID: "two"}, {VideoID: "three"}}
	got := OrderPlaylistItems(items, PlaylistOrderReverse)
	want := []string{"three", "two", "one"}
	for i, item := range got {
		if item.VideoID != want[i] {
			t.Fatalf("got[%d]=%q, want %q", i, item.VideoID, want[i])
		}
	}
	if items[0].VideoID != "one" {
		t.Fatalf("OrderPlaylistItems mutated input")
	}
}

func TestShufflePlaylistItems_DoesNotMutate(t *testing.T) {
	items := []PlaylistItem{{VideoID: "one"}, {VideoID: "two"}, {VideoID: "three"}, {VideoID: "four"}}
	got := ShufflePlaylistItems(items, rand.New(rand.NewSource(1)))
	if len(got) != len(items) {
		t.Fatalf("len=%d, want %d", len(got), len(items))
	}
	if items[0].VideoID != "one" || items[1].VideoID != "two" {
		t.Fatalf("ShufflePlaylistItems mutated input")
	}
}
