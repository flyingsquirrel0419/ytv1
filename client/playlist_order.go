package client

import (
	"math/rand"
	"time"
)

// PlaylistOrderMode controls playlist item ordering.
type PlaylistOrderMode int

const (
	PlaylistOrderOriginal PlaylistOrderMode = iota
	PlaylistOrderReverse
	PlaylistOrderRandom
)

// OrderPlaylistItems returns a non-mutating ordered copy of items.
func OrderPlaylistItems(items []PlaylistItem, mode PlaylistOrderMode) []PlaylistItem {
	if len(items) < 2 {
		return items
	}
	switch mode {
	case PlaylistOrderReverse:
		return ReversePlaylistItems(items)
	case PlaylistOrderRandom:
		return ShufflePlaylistItems(items, rand.New(rand.NewSource(time.Now().UnixNano())))
	default:
		return items
	}
}

// ReversePlaylistItems returns a reversed copy of items.
func ReversePlaylistItems(items []PlaylistItem) []PlaylistItem {
	out := append([]PlaylistItem(nil), items...)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// ShufflePlaylistItems returns a shuffled copy of items.
func ShufflePlaylistItems(items []PlaylistItem, rng *rand.Rand) []PlaylistItem {
	out := append([]PlaylistItem(nil), items...)
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	rng.Shuffle(len(out), func(i, j int) {
		out[i], out[j] = out[j], out[i]
	})
	return out
}
