package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/famomatic/ytv1/client"
	"github.com/famomatic/ytv1/internal/cli"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	fn()

	_ = w.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("io.Copy() error = %v", err)
	}
	return buf.String()
}

func TestRemediationHintsForAttempts_MissingPOT(t *testing.T) {
	hints := remediationHintsForAttempts([]client.AttemptDetail{
		{
			POTRequired:  true,
			POTAvailable: false,
		},
	})
	if len(hints) == 0 {
		t.Fatalf("expected hints for missing POT")
	}
	found := false
	for _, h := range hints {
		if strings.Contains(h, "--po-token") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected --po-token hint, got: %v", hints)
	}
}

func TestRemediationHintsForAttempts_403AndNoN(t *testing.T) {
	hints := remediationHintsForAttempts([]client.AttemptDetail{
		{
			HTTPStatus: 403,
			URLHasN:    false,
		},
	})
	found := false
	for _, h := range hints {
		if strings.Contains(h, "missing n-signature") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected n-signature hint, got: %v", hints)
	}
}

func TestPrintGenericRemediationHints_NoPlayableSelectorDetail(t *testing.T) {
	out := captureStdout(t, func() {
		printGenericRemediationHints(&client.NoPlayableFormatsDetailError{
			Mode:           client.SelectionModeBest,
			Selector:       "bestvideo+bestaudio",
			SelectionError: "no formats matched selector",
		})
	})
	if !strings.Contains(out, "bestvideo+bestaudio") || !strings.Contains(out, "matched no formats") {
		t.Fatalf("unexpected hint output: %q", out)
	}
}

func TestClassifyExitCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "invalid input", err: client.ErrInvalidInput, want: exitCodeInvalidInput},
		{name: "login required", err: client.ErrLoginRequired, want: exitCodeLoginRequired},
		{name: "unavailable", err: client.ErrUnavailable, want: exitCodeUnavailable},
		{name: "no playable", err: client.ErrNoPlayableFormats, want: exitCodeNoPlayableFormats},
		{name: "challenge", err: client.ErrChallengeNotSolved, want: exitCodeChallengeUnresolved},
		{name: "all clients", err: client.ErrAllClientsFailed, want: exitCodeAllClientsFailed},
		{name: "mp3", err: client.ErrMP3TranscoderNotConfigured, want: exitCodeMP3ConfigRequired},
		{name: "transcript parse", err: client.ErrTranscriptParse, want: exitCodeTranscriptParse},
		{name: "generic", err: errors.New("boom"), want: exitCodeGenericFailure},
	}
	for _, tt := range tests {
		got := classifyExitCode(tt.err)
		if got != tt.want {
			t.Fatalf("%s: classifyExitCode()=%d want=%d", tt.name, got, tt.want)
		}
	}
}

func TestEmitJSONFailure(t *testing.T) {
	out := captureStdout(t, func() {
		emitJSONFailure("jNQXAC9IVRw", &client.NoPlayableFormatsDetailError{
			Mode:           client.SelectionModeBest,
			Selector:       "bestvideo+bestaudio",
			SelectionError: "no formats matched selector",
		}, exitCodeNoPlayableFormats)
	})
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, payload=%q", err, out)
	}
	if ok, _ := payload["ok"].(bool); ok {
		t.Fatalf("expected ok=false payload")
	}
	if got := int(payload["exit_code"].(float64)); got != exitCodeNoPlayableFormats {
		t.Fatalf("exit_code=%d want=%d", got, exitCodeNoPlayableFormats)
	}
	errMap, _ := payload["error"].(map[string]any)
	if errMap["category"] != string(client.ErrorCategoryNoPlayableFormats) {
		t.Fatalf("error.category=%v", errMap["category"])
	}
}

func TestRun_PrintJSONStartupFailureForConfig(t *testing.T) {
	prevWriter := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(io.Discard)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
	}()

	out := captureStdout(t, func() {
		code := run(cli.Options{
			URLs:        []string{"jNQXAC9IVRw"},
			PrintJSON:   true,
			CookiesFile: filepath.Join(t.TempDir(), "missing-cookies.txt"),
		})
		if code != exitCodeGenericFailure {
			t.Fatalf("run() exit code=%d, want %d", code, exitCodeGenericFailure)
		}
	})

	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, payload=%q", err, out)
	}
	if ok, _ := payload["ok"].(bool); ok {
		t.Fatalf("expected ok=false payload")
	}
	errMap, _ := payload["error"].(map[string]any)
	msg, _ := errMap["message"].(string)
	if !strings.Contains(msg, "failed to initialize config") {
		t.Fatalf("unexpected startup error message: %q", msg)
	}
}
