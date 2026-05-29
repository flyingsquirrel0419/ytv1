package client

import (
	"context"
	"testing"
	"time"
)

func TestWithDefaultTimeoutShortensLongerDeadline(t *testing.T) {
	parent, cancelParent := context.WithTimeout(context.Background(), time.Hour)
	defer cancelParent()

	ctx, cancel := withDefaultTimeout(parent, 10*time.Millisecond)
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatalf("expected deadline")
	}
	if remaining := time.Until(deadline); remaining > time.Second {
		t.Fatalf("deadline remaining=%s, want request timeout to shorten parent deadline", remaining)
	}
}

func TestWithDefaultTimeoutKeepsShorterDeadline(t *testing.T) {
	parent, cancelParent := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancelParent()

	ctx, cancel := withDefaultTimeout(parent, time.Hour)
	defer cancel()

	parentDeadline, _ := parent.Deadline()
	gotDeadline, ok := ctx.Deadline()
	if !ok {
		t.Fatalf("expected deadline")
	}
	if !gotDeadline.Equal(parentDeadline) {
		t.Fatalf("deadline=%s, want parent deadline %s", gotDeadline, parentDeadline)
	}
}
