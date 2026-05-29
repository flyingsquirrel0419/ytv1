package client

import (
	"strings"
	"time"
)

// MediaFileMTime derives a media file modification time from YouTube metadata.
func MediaFileMTime(info *VideoInfo) (time.Time, bool) {
	if info == nil {
		return time.Time{}, false
	}
	raw := firstNonEmpty(info.UploadDate, info.PublishDate)
	for _, layout := range []string{"2006-01-02", "20060102"} {
		if t, err := time.ParseInLocation(layout, strings.TrimSpace(raw), time.UTC); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
