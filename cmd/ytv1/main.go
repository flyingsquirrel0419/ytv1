package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/famomatic/ytv1/client"
	"github.com/famomatic/ytv1/internal/cli"
	"github.com/famomatic/ytv1/internal/playerjs"
)

var verboseLifecyclePrinter *lifecyclePrinter
var activeDownloadArchive *client.DownloadArchive
var activeDownloadLimit *downloadLimit
var sleepBeforeRequestFunc = time.Sleep
var sleepBeforeDownloadFunc = time.Sleep
var sleepBeforeSubtitleFunc = time.Sleep

var errBreakOnExisting = errors.New("break on existing archive entry")
var errMaxDownloadsReached = errors.New("maximum number of downloads reached")

const (
	exitCodeSuccess             = 0
	exitCodeGenericFailure      = 1
	exitCodeInvalidInput        = 2
	exitCodeLoginRequired       = 3
	exitCodeUnavailable         = 4
	exitCodeNoPlayableFormats   = 5
	exitCodeChallengeUnresolved = 6
	exitCodeAllClientsFailed    = 7
	exitCodeDownloadFailed      = 8
	exitCodeMP3ConfigRequired   = 9
	exitCodeTranscriptParse     = 10
)

func main() {
	opts := cli.ParseFlags()
	os.Exit(run(opts))
}

func run(opts cli.Options) int {
	if len(opts.URLs) == 0 {
		err := fmt.Errorf("%w: no input URLs provided", client.ErrInvalidInput)
		if opts.PrintJSON {
			emitJSONFailure("", err, exitCodeInvalidInput)
		} else {
			fmt.Println("Usage: ytv1 [OPTIONS] URL [URL...]")
		}
		return exitCodeInvalidInput
	}

	cfg, err := cli.ToClientConfig(opts)
	if err != nil {
		return handleStartupError(opts, fmt.Errorf("failed to initialize config: %w", err))
	}
	if strings.TrimSpace(opts.DownloadArchive) != "" {
		archive, err := newDownloadArchive(opts.DownloadArchive)
		if err != nil {
			return handleStartupError(opts, fmt.Errorf("failed to initialize download archive: %w", err))
		}
		activeDownloadArchive = archive
		defer func() {
			if err := archive.Close(); err != nil {
				warnf(opts, "failed to close download archive: %v", err)
			}
		}()
	}
	if opts.MaxDownloads > 0 {
		activeDownloadLimit = &downloadLimit{Max: opts.MaxDownloads}
		defer func() {
			activeDownloadLimit = nil
		}()
	}
	attachLifecycleHandlers(&cfg, opts)
	c := client.New(cfg)
	ctx := context.Background()
	return processInputsWithExitCode(ctx, c, opts.URLs, opts, processURL)
}

func handleStartupError(opts cli.Options, err error) int {
	code := classifyExitCode(err)
	if opts.PrintJSON {
		emitJSONFailure("", err, code)
	} else {
		log.Printf("Error: %v", err)
	}
	return code
}

func processInputs(
	ctx context.Context,
	c *client.Client,
	urls []string,
	opts cli.Options,
	processor func(context.Context, *client.Client, string, cli.Options) error,
) bool {
	return processInputsWithExitCode(ctx, c, urls, opts, processor) != exitCodeSuccess
}

func processInputsWithExitCode(
	ctx context.Context,
	c *client.Client,
	urls []string,
	opts cli.Options,
	processor func(context.Context, *client.Client, string, cli.Options) error,
) int {
	exitCode := exitCodeSuccess
	for _, url := range urls {
		if err := processor(ctx, c, url, opts); err != nil {
			if errors.Is(err, errBreakOnExisting) || errors.Is(err, errMaxDownloadsReached) {
				break
			}
			code := classifyExitCode(err)
			if code > exitCode {
				exitCode = code
			}
			if opts.PrintJSON {
				emitJSONFailure(url, err, code)
			} else {
				log.Printf("Error processing %s: %v", url, err)
			}
			if (opts.OverrideDiagnostics || opts.Verbose) && !opts.PrintJSON {
				printAttemptDiagnostics(err)
			}
			if opts.AbortOnError {
				break
			}
		}
	}
	return exitCode
}

func attachLifecycleHandlers(cfg *client.Config, opts cli.Options) {
	if !opts.Verbose {
		return
	}
	lp := newLifecyclePrinter(time.Now)
	verboseLifecyclePrinter = lp
	cfg.OnExtractionEvent = func(evt client.ExtractionEvent) {
		fmt.Println(lp.formatExtractionEvent(evt))
	}
	cfg.OnDownloadEvent = func(evt client.DownloadEvent) {
		fmt.Println(lp.formatDownloadEvent(evt))
	}
}

func processURL(ctx context.Context, c *client.Client, url string, opts cli.Options) error {
	totalStart := time.Now()
	// 1. Check if it is a playlist
	// For now, treat everything as video unless we want to support playlists explicitly here
	// client.GetVideo handles video IDs.
	// Check prompt for playlist ID extraction
	if !opts.NoPlaylist {
		if playlistID, err := client.ExtractPlaylistID(url); err == nil && playlistID != "" {
			return processPlaylist(ctx, c, playlistID, opts)
		}
	}

	if opts.PlayerJSURLOnly {
		return handlePlayerJS(ctx, c, url)
	}
	if shouldSkipDownloadByArchive(url, opts) {
		if opts.BreakOnExisting {
			return errBreakOnExisting
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	extractStart := time.Now()
	if err := sleepBeforeExtractionRequest(ctx, opts); err != nil {
		return err
	}
	info, err := c.GetVideo(ctx, url)
	if err != nil {
		if opts.Verbose {
			fmt.Println(formatExtractionEvent(client.ExtractionEvent{
				Stage:  "total",
				Phase:  "failure",
				Client: "all",
				Detail: fmt.Sprintf("elapsed_ms=%d", time.Since(extractStart).Milliseconds()),
			}))
		}
		return err
	}
	extractMs := time.Since(extractStart).Milliseconds()
	if opts.Verbose {
		fmt.Println(formatExtractionEvent(client.ExtractionEvent{
			Stage:  "total",
			Phase:  "complete",
			Client: "all",
			Detail: fmt.Sprintf("elapsed_ms=%d", extractMs),
		}))
	}

	if opts.PrintJSON || opts.DumpSingleJSON {
		if err := emitDumpSingleJSON(os.Stdout, url, info); err != nil {
			return err
		}
		return recordForcedArchiveIfRequested(info, opts)
	}

	if opts.ListFormats {
		printFormats(info)
		return recordForcedArchiveIfRequested(info, opts) // yt-dlp stops after listing formats
	}
	if opts.ListSubs {
		if err := sleepBeforeExtractionRequest(ctx, opts); err != nil {
			return err
		}
		tracks, err := c.GetSubtitleTracks(ctx, url)
		if err != nil {
			return err
		}
		printSubtitleTracks(info, tracks)
		return recordForcedArchiveIfRequested(info, opts)
	}
	if opts.GetThumbnail {
		if err := printThumbnailURL(info); err != nil {
			return err
		}
		return recordForcedArchiveIfRequested(info, opts)
	}
	if hasMetadataGetFlag(opts) {
		resolveURL := func(itag int) (string, error) {
			return c.ResolveStreamURL(ctx, info.ID, itag)
		}
		if err := printRequestedMetadata(info, url, opts, resolveURL); err != nil {
			return err
		}
		return recordForcedArchiveIfRequested(info, opts)
	}

	if opts.WriteInfoJSON {
		if err := writeInfoJSONSidecar(url, info, opts); err != nil {
			return err
		}
	}
	if opts.WriteDescription {
		if err := writeDescriptionSidecar(info, opts); err != nil {
			return err
		}
	}
	if opts.WriteThumbnail {
		if err := writeThumbnailSidecar(ctx, c, info, opts); err != nil {
			return err
		}
	}
	if opts.WriteURLLink {
		if err := writeShortcutSidecar(url, info, opts, shortcutURL); err != nil {
			return err
		}
	}
	if opts.WriteWeblocLink {
		if err := writeShortcutSidecar(url, info, opts, shortcutWebloc); err != nil {
			return err
		}
	}
	if opts.WriteDesktopLink {
		if err := writeShortcutSidecar(url, info, opts, shortcutDesktop); err != nil {
			return err
		}
	}

	if opts.WriteSubs || opts.WriteAutoSubs {
		if err := writeRequestedSubtitles(ctx, c, url, info, opts); err != nil {
			return err
		}
	}

	if opts.SkipDownload {
		if shouldPrintHumanText(opts) {
			fmt.Printf("Skipping download for %s\n", info.Title)
		}
		return recordForcedArchiveIfRequested(info, opts)
	}

	if shouldPrintProgressText(opts) {
		fmt.Printf("Downloading: %s [%s]\n", info.Title, info.ID)
	}
	downloadOpts, err := buildDownloadOptionsForVideo(info, opts)
	if err != nil {
		return err
	}
	if shouldSkipExistingOutput(downloadOpts.OutputPath, opts) {
		if shouldPrintHumanText(opts) {
			fmt.Printf("Skipping existing file: %s\n", downloadOpts.OutputPath)
		}
		if err := recordCompletedDownload(info.ID); err != nil {
			return err
		}
		return nil
	}
	if shouldSkipExistingPostprocessedOutput(info, downloadOpts.OutputPath, opts) {
		if shouldPrintHumanText(opts) {
			fmt.Printf("Skipping existing post-processed file: %s\n", downloadOpts.OutputPath)
		}
		if err := recordCompletedDownload(info.ID); err != nil {
			return err
		}
		return nil
	}
	if err := sleepBeforeMediaDownload(ctx, opts); err != nil {
		return err
	}
	res, err := c.Download(ctx, url, downloadOpts)
	if err != nil {
		return err
	}
	if shouldPrintProgressText(opts) {
		fmt.Printf("Downloaded to: %s\n", res.OutputPath)
	}
	if err := applyDownloadedFileMTime(res.OutputPath, info, opts); err != nil {
		return err
	}
	if opts.Verbose && verboseLifecyclePrinter != nil {
		timing := verboseLifecyclePrinter.popVideoTiming(info.ID)
		videoMs := timing.downloadVideoMs
		audioMs := timing.downloadAudioMs
		if videoMs == 0 && audioMs == 0 && timing.downloadSingleMs > 0 {
			videoMs = timing.downloadSingleMs
		}
		downloadTotalMs := videoMs + audioMs
		avgSpeed := "0B/s"
		if downloadTotalMs > 0 {
			bps := int64(float64(res.Bytes) / (float64(downloadTotalMs) / 1000.0))
			avgSpeed = fmt.Sprintf("%dB/s", bps)
		}
		if shouldPrintHumanText(opts) {
			fmt.Printf(
				"total_elapsed_ms=%d extract_ms=%d download_ms(video/audio)=%d/%d merge_ms=%d final_size=%d avg_speed=%s\n",
				time.Since(totalStart).Milliseconds(),
				extractMs,
				videoMs,
				audioMs,
				timing.mergeMs,
				res.Bytes,
				avgSpeed,
			)
		}
	}
	if err := recordCompletedDownload(info.ID); err != nil {
		return err
	}
	return nil
}

func shouldSkipExistingOutput(path string, opts cli.Options) bool {
	if !opts.NoOverwrites || strings.TrimSpace(path) == "" {
		return false
	}
	if _, err := os.Stat(path); err == nil {
		return true
	}
	return false
}

func shouldSkipExistingPostprocessedOutput(info *client.VideoInfo, path string, opts cli.Options) bool {
	if !opts.NoPostOverwrites || strings.TrimSpace(path) == "" {
		return false
	}
	if !isPostprocessedOutput(info, opts) {
		return false
	}
	if _, err := os.Stat(path); err == nil {
		return true
	}
	return false
}

func isPostprocessedOutput(info *client.VideoInfo, opts cli.Options) bool {
	downloadOpts := buildDownloadOptions(opts)
	if downloadOpts.Mode == client.SelectionModeMP3 {
		return true
	}
	if info == nil {
		return false
	}
	formats, err := selectedFormatsForOptions(info.Formats, opts)
	if err != nil {
		return false
	}
	return isMergedSelection(formats)
}

func sleepBeforeExtractionRequest(ctx context.Context, opts cli.Options) error {
	if opts.SleepRequests <= 0 {
		return nil
	}
	return sleepWithContext(ctx, opts.SleepRequests, sleepBeforeRequestFunc)
}

func sleepBeforeMediaDownload(ctx context.Context, opts cli.Options) error {
	d := mediaDownloadSleepDuration(opts)
	if d <= 0 {
		return nil
	}
	return sleepWithContext(ctx, d, sleepBeforeDownloadFunc)
}

func mediaDownloadSleepDuration(opts cli.Options) time.Duration {
	minSleep := opts.SleepInterval
	if minSleep <= 0 {
		return 0
	}
	if opts.MaxSleepInterval > minSleep {
		return minSleep
	}
	return minSleep
}

func sleepBeforeSubtitleDownload(ctx context.Context, opts cli.Options) error {
	if opts.SleepSubtitles <= 0 {
		return nil
	}
	return sleepWithContext(ctx, opts.SleepSubtitles, sleepBeforeSubtitleFunc)
}

func sleepWithContext(ctx context.Context, d time.Duration, sleepFn func(time.Duration)) error {
	if d <= 0 {
		return nil
	}
	done := make(chan struct{})
	go func() {
		sleepFn(d)
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

func applyDownloadedFileMTime(path string, info *client.VideoInfo, opts cli.Options) error {
	if !opts.UpdateMTime || strings.TrimSpace(path) == "" {
		return nil
	}
	mt, ok := client.MediaFileMTime(info)
	if !ok {
		return nil
	}
	if err := os.Chtimes(path, mt, mt); err != nil {
		return fmt.Errorf("failed to update media mtime: %w", err)
	}
	return nil
}

func buildDownloadOptions(opts cli.Options) client.DownloadOptions {
	downloadOpts := client.DownloadOptions{
		Mode:                  client.SelectionModeBest,
		OutputPath:            effectiveOutputTemplate(opts),
		MergeOutput:           true, // Always try to merge on 'best'
		KeepIntermediateFiles: opts.KeepVideo,
		Resume:                !opts.NoContinue,
		UsePartFiles:          !opts.NoPart,
		AudioQuality:          strings.TrimSpace(opts.AudioQuality),
		NoEmbedMetadata:       !opts.EmbedMetadata,
		MergeOutputFormat:     effectiveCLIMergeOutputExt(opts),
	}

	raw := strings.TrimSpace(opts.FormatSelector)
	if opts.ExtractAudio && (raw == "" || strings.EqualFold(raw, "best")) {
		switch strings.ToLower(strings.TrimSpace(opts.AudioFormat)) {
		case "mp3":
			raw = "mp3"
		default:
			raw = "bestaudio"
		}
	}
	lower := strings.ToLower(raw)
	switch lower {
	case "", "best":
		return downloadOpts
	case "bestvideo+bestaudio":
		downloadOpts.FormatSelector = "bestvideo+bestaudio/best"
		return downloadOpts
	case "bestaudio", "audioonly":
		downloadOpts.Mode = client.SelectionModeAudioOnly
		return downloadOpts
	case "bestvideo", "videoonly":
		downloadOpts.Mode = client.SelectionModeVideoOnly
		return downloadOpts
	case "mp4":
		downloadOpts.Mode = client.SelectionModeMP4AV
		return downloadOpts
	case "mp3":
		downloadOpts.Mode = client.SelectionModeMP3
		return downloadOpts
	}

	if itag, err := strconv.Atoi(lower); err == nil && itag > 0 {
		downloadOpts.Itag = itag
		return downloadOpts
	}

	downloadOpts.FormatSelector = raw
	return downloadOpts
}

func buildDownloadOptionsForVideo(info *client.VideoInfo, opts cli.Options) (client.DownloadOptions, error) {
	downloadOpts := buildDownloadOptions(opts)
	if info == nil {
		return downloadOpts, nil
	}
	outputPath, err := predictedOutputFilename(info, opts)
	if err != nil {
		return client.DownloadOptions{}, err
	}
	downloadOpts.OutputPath = outputPath
	return downloadOpts, nil
}

func effectiveOutputTemplate(opts cli.Options) string {
	outputTemplate := strings.TrimSpace(opts.OutputTemplate)
	if opts.OutputUseID {
		outputTemplate = "%(id)s.%(ext)s"
	}
	outputDir := strings.TrimSpace(opts.OutputPathDir)
	if outputDir == "" {
		return outputTemplate
	}
	if outputTemplate == "" {
		return filepath.Join(outputDir, "%(id)s-%(itag)s.%(ext)s")
	}
	if filepath.IsAbs(outputTemplate) {
		return outputTemplate
	}
	return filepath.Join(outputDir, outputTemplate)
}

func processPlaylist(ctx context.Context, c *client.Client, playlistID string, opts cli.Options) error {
	if shouldPrintPlaylistText(opts) {
		fmt.Printf("Fetching playlist: %s\n", playlistID)
	}
	if err := sleepBeforeExtractionRequest(ctx, opts); err != nil {
		return err
	}
	playlist, err := c.GetPlaylist(ctx, playlistID)
	if err != nil {
		return err
	}
	if shouldPrintPlaylistText(opts) {
		fmt.Printf("Playlist: %s (%d videos)\n", playlist.Title, len(playlist.Items))
	}
	if opts.WriteInfoJSON && !opts.NoWritePlaylistMetafiles {
		if err := writePlaylistInfoJSONSidecar(playlist, opts); err != nil {
			return err
		}
	}
	itemsWithIndex := withPlaylistIndexes(playlist.Items)
	items, err := client.SelectPlaylistItems(itemsWithIndex, playlistSelector(opts))
	if err != nil {
		return err
	}
	items = orderPlaylistItems(items, opts)
	if opts.FlatPlaylist {
		return emitFlatPlaylist(items, opts, os.Stdout)
	}

	playlistCtx := playlistTemplateContext{
		ID:         playlist.ID,
		Title:      playlist.Title,
		Uploader:   playlist.Uploader,
		UploaderID: playlist.UploaderID,
		Channel:    playlist.Channel,
		ChannelID:  playlist.ChannelID,
		Count:      len(items),
	}
	summary, failures := runPlaylistItems(ctx, c, items, playlistCtx, opts, processURL)
	if shouldPrintPlaylistText(opts) {
		fmt.Printf(
			"Playlist summary: total=%d succeeded=%d failed=%d skipped=%d aborted=%t\n",
			summary.Total,
			summary.Succeeded,
			summary.Failed,
			summary.Skipped,
			summary.Aborted,
		)
	}
	if len(failures) > 0 {
		if shouldPrintPlaylistText(opts) {
			for _, failure := range failures {
				log.Printf("Failed to process %s: %v", failure.VideoID, failure.Err)
			}
		}
		return fmt.Errorf("playlist completed with failures: failed=%d/%d", summary.Failed, summary.Total)
	}
	return nil
}

func playlistSelector(opts cli.Options) string {
	if strings.TrimSpace(opts.PlaylistItems) != "" {
		return opts.PlaylistItems
	}
	if opts.PlaylistStart <= 0 && opts.PlaylistEnd <= 0 {
		return ""
	}
	start := ""
	end := ""
	if opts.PlaylistStart > 0 {
		start = strconv.Itoa(opts.PlaylistStart)
	}
	if opts.PlaylistEnd > 0 {
		end = strconv.Itoa(opts.PlaylistEnd)
	}
	return start + ":" + end
}

func orderPlaylistItems(items []client.PlaylistItem, opts cli.Options) []client.PlaylistItem {
	if opts.PlaylistReverse {
		return client.OrderPlaylistItems(items, client.PlaylistOrderReverse)
	}
	if opts.PlaylistRandom {
		return client.OrderPlaylistItems(items, client.PlaylistOrderRandom)
	}
	return items
}

func reversePlaylistItems(items []client.PlaylistItem) []client.PlaylistItem {
	return client.ReversePlaylistItems(items)
}

func shufflePlaylistItems(items []client.PlaylistItem, rng *rand.Rand) []client.PlaylistItem {
	return client.ShufflePlaylistItems(items, rng)
}

func shouldPrintPlaylistText(opts cli.Options) bool {
	return shouldPrintHumanText(opts)
}

func shouldPrintHumanText(opts cli.Options) bool {
	return !opts.PrintJSON && !opts.DumpSingleJSON && !opts.Quiet
}

func shouldPrintProgressText(opts cli.Options) bool {
	if opts.PrintJSON || opts.DumpSingleJSON {
		return false
	}
	if opts.NoProgress {
		return false
	}
	return opts.Progress || !opts.Quiet
}

func emitFlatPlaylist(items []client.PlaylistItem, opts cli.Options, w io.Writer) error {
	if opts.PrintJSON {
		enc := json.NewEncoder(w)
		for _, item := range items {
			payload := map[string]any{
				"_type": "url",
				"id":    item.VideoID,
				"title": item.Title,
				"url":   "https://www.youtube.com/watch?v=" + item.VideoID,
			}
			if err := enc.Encode(payload); err != nil {
				return err
			}
		}
		return nil
	}
	for _, item := range items {
		if _, err := fmt.Fprintf(w, "[flat] %s\t%s\n", item.VideoID, item.Title); err != nil {
			return err
		}
	}
	return nil
}

type playlistRunSummary struct {
	Total     int
	Succeeded int
	Failed    int
	Aborted   bool
	Skipped   int
}

type playlistItemFailure struct {
	VideoID string
	Err     error
}

type playlistTemplateContext struct {
	ID         string
	Title      string
	Uploader   string
	UploaderID string
	Channel    string
	ChannelID  string
	Count      int
}

func runPlaylistItems(
	ctx context.Context,
	c *client.Client,
	items []client.PlaylistItem,
	playlistCtx playlistTemplateContext,
	opts cli.Options,
	processor func(context.Context, *client.Client, string, cli.Options) error,
) (playlistRunSummary, []playlistItemFailure) {
	summary := playlistRunSummary{Total: len(items)}
	failures := make([]playlistItemFailure, 0)
	for i, item := range items {
		if shouldPrintProgressText(opts) {
			fmt.Printf("[%d/%d] Processing %s (%s)...\n", i+1, len(items), item.Title, item.VideoID)
		}
		itemOpts := applyPlaylistTemplateContext(opts, item, i+1, playlistCtx)
		if err := processor(ctx, c, item.VideoID, itemOpts); err != nil {
			if errors.Is(err, errBreakOnExisting) {
				summary.Aborted = true
				summary.Skipped = len(items) - i - 1
				break
			}
			if errors.Is(err, errMaxDownloadsReached) {
				summary.Succeeded++
				summary.Aborted = true
				summary.Skipped = len(items) - i - 1
				break
			}
			summary.Failed++
			failures = append(failures, playlistItemFailure{
				VideoID: item.VideoID,
				Err:     err,
			})
			if opts.AbortOnError || reachedPlaylistFailureThreshold(summary.Failed, opts.SkipPlaylistAfterErrors) {
				summary.Aborted = true
				summary.Skipped = len(items) - i - 1
				break
			}
			continue
		}
		summary.Succeeded++
	}
	return summary, failures
}

func withPlaylistIndexes(items []client.PlaylistItem) []client.PlaylistItem {
	out := append([]client.PlaylistItem(nil), items...)
	for i := range out {
		if out[i].PlaylistIndex <= 0 {
			out[i].PlaylistIndex = i + 1
		}
	}
	return out
}

func applyPlaylistTemplateContext(opts cli.Options, item client.PlaylistItem, autonumber int, playlistCtx playlistTemplateContext) cli.Options {
	if strings.TrimSpace(opts.OutputTemplate) == "" {
		return opts
	}
	index := item.PlaylistIndex
	if index <= 0 {
		index = autonumber
	}
	replacements := map[string]string{
		"%(playlist_index)s":       fmt.Sprintf("%05d", index),
		"%(playlist_autonumber)s":  fmt.Sprintf("%05d", autonumber),
		"%(playlist_count)s":       fmt.Sprintf("%05d", playlistCtx.Count),
		"%(playlist_id)s":          sanitizeTemplateToken(playlistCtx.ID),
		"%(playlist_title)s":       sanitizeTemplateToken(playlistCtx.Title),
		"%(playlist_uploader)s":    sanitizeTemplateToken(playlistCtx.Uploader),
		"%(playlist_uploader_id)s": sanitizeTemplateToken(playlistCtx.UploaderID),
		"%(playlist_channel)s":     sanitizeTemplateToken(playlistCtx.Channel),
		"%(playlist_channel_id)s":  sanitizeTemplateToken(playlistCtx.ChannelID),
	}
	for token, value := range replacements {
		opts.OutputTemplate = strings.ReplaceAll(opts.OutputTemplate, token, value)
	}
	return opts
}

func reachedPlaylistFailureThreshold(failures int, threshold int) bool {
	return threshold > 0 && failures >= threshold
}

func printFormats(info *client.VideoInfo) {
	fmt.Printf("Title: %s\n", info.Title)
	fmt.Println("ID | Ext | Resolution | FPS | Bitrate | Proto | Codec | Note")
	fmt.Println("---|-----|------------|-----|---------|-------|-------|------")
	for _, f := range info.Formats {
		fmt.Printf("%3d|%4s|%4dx%-4d|%3d|%6dk|%5s|%s|%s\n",
			f.Itag, client.FormatMediaExt(f.MimeType), f.Width, f.Height, f.FPS, f.Bitrate/1000, f.Protocol, f.MimeType, client.FormatTrackNote(f))
	}
}

func printSubtitleTracks(info *client.VideoInfo, tracks []client.SubtitleTrack) {
	fmt.Printf("Available subtitles for %s [%s]\n", info.Title, info.ID)
	fmt.Println("Lang | Name | Ext | Type")
	fmt.Println("-----|------|-----|-----")
	for _, track := range tracks {
		kind := "manual"
		if track.AutoGenerated {
			kind = "auto"
		}
		ext := track.Ext
		if strings.TrimSpace(ext) == "" {
			ext = "vtt"
		}
		fmt.Printf("%s|%s|%s|%s\n", track.LanguageCode, track.Name, ext, kind)
	}
}

func printThumbnailURL(info *client.VideoInfo) error {
	if info == nil || strings.TrimSpace(info.ThumbnailURL) == "" {
		videoID := ""
		if info != nil {
			videoID = info.ID
		}
		return fmt.Errorf("%w: thumbnail unavailable for video=%s", client.ErrUnavailable, videoID)
	}
	fmt.Println(info.ThumbnailURL)
	return nil
}

func hasMetadataGetFlag(opts cli.Options) bool {
	return opts.GetTitle || opts.GetID || opts.GetDescription || opts.GetDuration || opts.GetFormat || opts.GetFilename || opts.GetURL || len(opts.PrintTemplates) > 0 || len(opts.PrintToFile) > 0 || len(opts.GetMetadataOrder) > 0
}

type streamURLResolver func(itag int) (string, error)

func printRequestedMetadata(info *client.VideoInfo, input string, opts cli.Options, resolveURL streamURLResolver) error {
	order := opts.GetMetadataOrder
	if len(order) == 0 {
		if opts.GetTitle {
			order = append(order, "title")
		}
		if opts.GetID {
			order = append(order, "id")
		}
		if opts.GetDescription {
			order = append(order, "description")
		}
		if opts.GetDuration {
			order = append(order, "duration")
		}
		if opts.GetFilename {
			order = append(order, "filename")
		}
		if opts.GetFormat {
			order = append(order, "format")
		}
		if opts.GetURL {
			order = append(order, "url")
		}
		for _, template := range opts.PrintTemplates {
			order = append(order, "print:"+template)
		}
		for _, spec := range opts.PrintToFile {
			order = append(order, "printfile:"+spec.Template+"\x00"+spec.File)
		}
	}
	for _, field := range order {
		switch field {
		case "title":
			fmt.Println(info.Title)
		case "id":
			fmt.Println(info.ID)
		case "description":
			fmt.Println(info.Description)
		case "duration":
			fmt.Println(client.FormatDuration(info.DurationSec))
		case "filename":
			filename, err := predictedOutputFilename(info, opts)
			if err != nil {
				return err
			}
			fmt.Println(filename)
		case "format":
			formats, err := selectedFormatsForOptions(info.Formats, opts)
			if err != nil {
				return err
			}
			fmt.Println(formatSelectedFormats(formats))
		case "url":
			urls, err := selectedStreamURLs(info, opts, resolveURL)
			if err != nil {
				return err
			}
			for _, url := range urls {
				fmt.Println(url)
			}
		default:
			if strings.HasPrefix(field, "print:") {
				line, err := renderPrintTemplate(strings.TrimPrefix(field, "print:"), info, input, opts, resolveURL)
				if err != nil {
					return err
				}
				fmt.Println(line)
			} else if strings.HasPrefix(field, "printfile:") {
				spec, ok := parsePrintFileOrderField(strings.TrimPrefix(field, "printfile:"))
				if !ok {
					continue
				}
				if err := appendPrintToFile(spec, info, input, opts, resolveURL); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func parsePrintFileOrderField(raw string) (cli.PrintToFileSpec, bool) {
	parts := strings.SplitN(raw, "\x00", 2)
	if len(parts) != 2 {
		return cli.PrintToFileSpec{}, false
	}
	return cli.PrintToFileSpec{Template: parts[0], File: parts[1]}, true
}

func appendPrintToFile(spec cli.PrintToFileSpec, info *client.VideoInfo, input string, opts cli.Options, resolveURL streamURLResolver) error {
	line, err := renderPrintTemplate(spec.Template, info, input, opts, resolveURL)
	if err != nil {
		return err
	}
	path := strings.TrimSpace(spec.File)
	if path == "" {
		return fmt.Errorf("%w: empty --print-to-file path", client.ErrInvalidInput)
	}
	path = renderCLITemplate(path, cliTemplateData{
		VideoID:           info.ID,
		Title:             info.Title,
		Uploader:          info.Author,
		UploaderID:        cliTemplateUploaderID(info),
		Channel:           info.Author,
		ChannelID:         info.ChannelID,
		Ext:               "txt",
		Itag:              "print",
		FormatID:          "print",
		UploadDate:        cliTemplateUploadDate(info),
		ReleaseDate:       cliTemplateReleaseDate(info),
		Timestamp:         cliTemplateTimestamp(info),
		RestrictFilenames: opts.RestrictFilenames,
	})
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create print-to-file directory: %w", err)
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open print-to-file output: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(line + "\n"); err != nil {
		return fmt.Errorf("failed to write print-to-file output: %w", err)
	}
	return nil
}

func renderPrintTemplate(template string, info *client.VideoInfo, input string, opts cli.Options, resolveURL streamURLResolver) (string, error) {
	template = strings.TrimSpace(client.StripPrintStagePrefix(template))
	filename, err := predictedOutputFilename(info, opts)
	if err != nil && (template == "filename" || strings.Contains(template, "%(filename)s")) {
		return "", err
	}
	formatSummary := ""
	if strings.Contains(template, "%(format)s") {
		formats, err := selectedFormatsForOptions(info.Formats, opts)
		if err != nil {
			return "", err
		}
		formatSummary = formatSelectedFormats(formats)
	}
	streamURL := ""
	if strings.Contains(template, "%(url)s") {
		urls, err := selectedStreamURLs(info, opts, resolveURL)
		if err != nil {
			return "", err
		}
		streamURL = strings.Join(urls, "\n")
	}
	switch template {
	case "filename":
		return filename, nil
	case "format":
		return formatSummaryForPrint(info, opts)
	case "url":
		urls, err := selectedStreamURLs(info, opts, resolveURL)
		if err != nil {
			return "", err
		}
		return strings.Join(urls, "\n"), nil
	}
	formats, err := selectedFormatsForOptions(info.Formats, opts)
	ext := ""
	itag := ""
	formatTokens := client.FormatTemplateTokens{}
	if err == nil && len(formats) > 0 {
		if isMergedSelection(formats) {
			videoItag, audioItag := mergedItags(formats)
			ext = effectiveCLIMergeOutputExt(opts)
			itag = fmt.Sprintf("%d+%d", videoItag, audioItag)
		} else {
			ext = detectCLIOutputExt(formats[0].MimeType, buildDownloadOptions(opts).Mode)
			itag = strconv.Itoa(formats[0].Itag)
		}
		formatTokens = selectedFormatTemplateTokens(formats)
	}
	return client.RenderMetadataPrintTemplate(template, client.MetadataPrintData{
		Info:          info,
		Input:         input,
		Filename:      filename,
		FormatSummary: formatSummary,
		URL:           streamURL,
		ThumbnailURL:  info.ThumbnailURL,
		Description:   info.Description,
		Duration:      client.FormatDuration(info.DurationSec),
		UploadDate:    cliTemplateUploadDate(info),
		ReleaseDate:   cliTemplateReleaseDate(info),
		Timestamp:     cliTemplateTimestamp(info),
		OutputTemplate: client.OutputTemplateData{
			VideoID:           info.ID,
			Title:             info.Title,
			Uploader:          info.Author,
			UploaderID:        cliTemplateUploaderID(info),
			Channel:           info.Author,
			ChannelID:         info.ChannelID,
			Ext:               ext,
			Itag:              itag,
			FormatID:          itag,
			Resolution:        formatTokens.Resolution,
			Width:             formatTokens.Width,
			Height:            formatTokens.Height,
			FPS:               formatTokens.FPS,
			TBR:               formatTokens.TBR,
			VBR:               formatTokens.VBR,
			ABR:               formatTokens.ABR,
			Protocol:          formatTokens.Protocol,
			VCodec:            formatTokens.VCodec,
			ACodec:            formatTokens.ACodec,
			UploadDate:        cliTemplateUploadDate(info),
			ReleaseDate:       cliTemplateReleaseDate(info),
			Timestamp:         cliTemplateTimestamp(info),
			RestrictFilenames: opts.RestrictFilenames,
		},
	})
}

func formatSummaryForPrint(info *client.VideoInfo, opts cli.Options) (string, error) {
	formats, err := selectedFormatsForOptions(info.Formats, opts)
	if err != nil {
		return "", err
	}
	return formatSelectedFormats(formats), nil
}

func normalizedCLIMergeOutputExt(raw string) string {
	return client.NormalizeMergeOutputExt(raw)
}

func effectiveCLIMergeOutputExt(opts cli.Options) string {
	if strings.TrimSpace(opts.MergeOutputFormat) != "" {
		return normalizedCLIMergeOutputExt(opts.MergeOutputFormat)
	}
	return normalizedCLIMergeOutputExt(remuxVideoTargetExt(opts.RemuxVideo))
}

func remuxVideoTargetExt(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parts := strings.Split(raw, "/")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if i := strings.LastIndex(part, ">"); i >= 0 {
			part = strings.TrimSpace(part[i+1:])
		}
		if part != "" {
			return part
		}
	}
	return raw
}

func predictedOutputFilename(info *client.VideoInfo, opts cli.Options) (string, error) {
	formats, err := selectedFormatsForOptions(info.Formats, opts)
	if err != nil {
		return "", err
	}
	downloadOpts := buildDownloadOptions(opts)
	return client.PredictOutputFilename(info, formats, client.OutputFilenameOptions{
		OutputTemplate:    downloadOpts.OutputPath,
		Mode:              downloadOpts.Mode,
		MergeOutputExt:    effectiveCLIMergeOutputExt(opts),
		TrimFilenames:     opts.TrimFilenames,
		RestrictFilenames: opts.RestrictFilenames,
	})
}

func selectedStreamURLs(info *client.VideoInfo, opts cli.Options, resolveURL streamURLResolver) ([]string, error) {
	formats, err := selectedFormatsForOptions(info.Formats, opts)
	if err != nil {
		return nil, err
	}
	urls := make([]string, 0, len(formats))
	for _, format := range formats {
		if strings.TrimSpace(format.URL) != "" && !format.Ciphered {
			urls = append(urls, format.URL)
			continue
		}
		if resolveURL == nil {
			return nil, fmt.Errorf("%w: direct URL unavailable for itag=%d", client.ErrUnavailable, format.Itag)
		}
		resolved, err := resolveURL(format.Itag)
		if err != nil {
			return nil, fmt.Errorf("resolve stream URL for itag=%d: %w", format.Itag, err)
		}
		if strings.TrimSpace(resolved) == "" {
			return nil, fmt.Errorf("%w: empty stream URL for itag=%d", client.ErrUnavailable, format.Itag)
		}
		urls = append(urls, resolved)
	}
	return urls, nil
}

func isMergedSelection(formats []client.FormatInfo) bool {
	return client.IsMergedSelection(formats)
}

func mergedItags(formats []client.FormatInfo) (int, int) {
	return client.MergedItags(formats)
}

type cliTemplateData = client.OutputTemplateData

func renderCLITemplate(template string, data cliTemplateData) string {
	return client.RenderOutputTemplate(template, data)
}

func selectedFormatTemplateTokens(formats []client.FormatInfo) client.FormatTemplateTokens {
	return client.SelectedFormatTemplateTokens(formats)
}

func cliTemplateUploaderID(info *client.VideoInfo) string {
	if info == nil {
		return ""
	}
	return info.ChannelID
}

func cliTemplateUploadDate(info *client.VideoInfo) string {
	if info == nil {
		return ""
	}
	return compactDateToken(firstNonEmpty(info.UploadDate, info.PublishDate))
}

func cliTemplateReleaseDate(info *client.VideoInfo) string {
	if info == nil {
		return ""
	}
	return compactDateToken(firstNonEmpty(info.PublishDate, info.UploadDate))
}

func cliTemplateTimestamp(info *client.VideoInfo) string {
	if info == nil {
		return ""
	}
	date := compactDateToken(firstNonEmpty(info.UploadDate, info.PublishDate))
	if len(date) != 8 {
		return ""
	}
	return date + "000000"
}

func compactDateToken(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func defaultCLIOutputPath(videoID string, itag int, mimeType string, mode client.SelectionMode) string {
	return client.DefaultOutputPath(videoID, itag, mimeType, mode)
}

func detectCLIOutputExt(mimeType string, mode client.SelectionMode) string {
	return client.DetectOutputExt(mimeType, mode)
}

func selectedFormatsForOptions(formats []client.FormatInfo, opts cli.Options) ([]client.FormatInfo, error) {
	return client.SelectFormatsForDownloadOptions(formats, buildDownloadOptions(opts))
}

func formatSelectedFormats(formats []client.FormatInfo) string {
	return client.FormatSummaries(formats)
}

func handlePlayerJS(ctx context.Context, c *client.Client, videoID string) error {
	resolver := playerjs.NewResolver(nil, playerjs.NewMemoryCache())
	path, err := resolver.GetPlayerURL(ctx, videoID)
	if err != nil {
		return err
	}
	if strings.HasPrefix(path, "http") {
		fmt.Println(path)
	} else {
		fmt.Println("https://www.youtube.com" + path)
	}
	return nil
}

func writeRequestedSubtitles(
	ctx context.Context,
	c *client.Client,
	input string,
	info *client.VideoInfo,
	opts cli.Options,
) error {
	subFormat := client.ResolveSubtitleOutputFormat(opts.SubFormat)
	langs := parseSubtitleLanguages(opts.SubLangs)
	if opts.AllSubs || subtitleLanguagesRequestAll(langs) {
		if err := sleepBeforeExtractionRequest(ctx, opts); err != nil {
			return err
		}
		tracks, err := c.GetSubtitleTracks(ctx, input)
		if err != nil {
			return err
		}
		langs = subtitleLanguagesFromTracksForRequest(tracks, opts, langs)
	} else {
		langs = applySubtitleLanguageExclusions(langs)
	}
	if len(langs) == 0 {
		langs = []string{"en"}
	}

	written := 0
	failures := make([]string, 0, len(langs))
	for _, lang := range langs {
		if err := sleepBeforeSubtitleDownload(ctx, opts); err != nil {
			return err
		}
		transcript, err := c.GetTranscript(ctx, input, lang)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s(%v)", lang, err))
			continue
		}
		outputPath := subtitleOutputPath(effectiveOutputTemplate(opts), info, transcript.LanguageCode, string(subFormat), opts.RestrictFilenames, opts.TrimFilenames)
		if shouldSkipExistingOutput(outputPath, opts) {
			if shouldPrintHumanText(opts) {
				fmt.Printf("Skipping existing subtitle: %s\n", outputPath)
			}
			continue
		}
		if err := client.WriteTranscript(outputPath, transcript, subFormat); err != nil {
			failures = append(failures, fmt.Sprintf("%s(%v)", transcript.LanguageCode, err))
			continue
		}
		written++
		if shouldPrintHumanText(opts) {
			fmt.Printf("Written subtitle: %s\n", outputPath)
		}
	}

	if written == 0 && len(failures) > 0 {
		return fmt.Errorf("failed to write subtitles: %s", strings.Join(failures, "; "))
	}
	if len(failures) > 0 {
		warnf(opts, "subtitle partial failure: %s", strings.Join(failures, "; "))
	}
	return nil
}

func warnf(opts cli.Options, format string, args ...any) {
	if opts.NoWarnings || opts.Quiet {
		return
	}
	log.Printf("WARNING: "+format, args...)
}

func parseSubtitleLanguages(raw string) []string {
	return client.ParseSubtitleLanguages(raw)
}

func subtitleLanguagesRequestAll(langs []string) bool {
	return client.SubtitleLanguagesRequestAll(langs)
}

func applySubtitleLanguageExclusions(langs []string) []string {
	return client.ApplySubtitleLanguageExclusions(langs)
}

func subtitleLanguagesFromTracksForRequest(tracks []client.SubtitleTrack, opts cli.Options, requested []string) []string {
	return client.SubtitleLanguagesFromTracksForRequest(tracks, requested, subtitleLanguageSelection(opts))
}

func subtitleLanguagesFromTracks(tracks []client.SubtitleTrack, opts cli.Options) []string {
	return client.SubtitleLanguagesFromTracks(tracks, subtitleLanguageSelection(opts))
}

func subtitleLanguageSelection(opts cli.Options) client.SubtitleLanguageSelection {
	return client.SubtitleLanguageSelection{
		IncludeManual: opts.WriteSubs || !opts.WriteAutoSubs,
		IncludeAuto:   opts.WriteAutoSubs,
	}
}

func subtitleOutputPath(outputTemplate string, info *client.VideoInfo, lang string, outputExt string, restrictFilenamesAndTrim ...any) string {
	restrict, trimLimit := templatePathOptions(restrictFilenamesAndTrim...)
	return client.SubtitleOutputPath(info, lang, outputExt, client.SidecarPathOptions{
		OutputTemplate:    outputTemplate,
		RestrictFilenames: restrict,
		TrimFilenames:     trimLimit,
	})
}

func templatePathOptions(values ...any) (bool, int) {
	restrict := false
	trimLimit := 0
	if len(values) > 0 {
		if v, ok := values[0].(bool); ok {
			restrict = v
		}
	}
	if len(values) > 1 {
		if v, ok := values[1].(int); ok {
			trimLimit = v
		}
	}
	return restrict, trimLimit
}

func trimOutputPathFilename(path string, limit int) string {
	return client.TrimOutputPathFilename(path, limit)
}

func writeInfoJSONSidecar(input string, info *client.VideoInfo, opts cli.Options) error {
	outputPath := infoJSONOutputPath(effectiveOutputTemplate(opts), info, opts.RestrictFilenames, opts.TrimFilenames)
	if shouldSkipExistingOutput(outputPath, opts) {
		if shouldPrintHumanText(opts) {
			fmt.Printf("Skipping existing info JSON: %s\n", outputPath)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil && filepath.Dir(outputPath) != "." {
		return fmt.Errorf("failed to create info json directory: %w", err)
	}
	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create info json: %w", err)
	}
	defer f.Close()

	if err := emitDumpSingleJSON(f, input, info); err != nil {
		return fmt.Errorf("failed to write info json: %w", err)
	}
	if shouldPrintHumanText(opts) {
		fmt.Printf("Written info JSON: %s\n", outputPath)
	}
	return nil
}

func writePlaylistInfoJSONSidecar(playlist *client.PlaylistInfo, opts cli.Options) error {
	info := playlistInfoAsVideoInfo(playlist)
	outputPath := infoJSONOutputPath(effectiveOutputTemplate(opts), info, opts.RestrictFilenames, opts.TrimFilenames)
	if shouldSkipExistingOutput(outputPath, opts) {
		if shouldPrintHumanText(opts) {
			fmt.Printf("Skipping existing playlist info JSON: %s\n", outputPath)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil && filepath.Dir(outputPath) != "." {
		return fmt.Errorf("failed to create playlist info json directory: %w", err)
	}
	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create playlist info json: %w", err)
	}
	defer f.Close()

	if err := emitPlaylistInfoJSON(f, playlist); err != nil {
		return fmt.Errorf("failed to write playlist info json: %w", err)
	}
	if shouldPrintHumanText(opts) {
		fmt.Printf("Written playlist info JSON: %s\n", outputPath)
	}
	return nil
}

func playlistInfoAsVideoInfo(playlist *client.PlaylistInfo) *client.VideoInfo {
	if playlist == nil {
		return &client.VideoInfo{}
	}
	author := firstNonEmpty(playlist.Uploader, playlist.Channel)
	return &client.VideoInfo{
		ID:        playlist.ID,
		Title:     playlist.Title,
		Author:    author,
		ChannelID: firstNonEmpty(playlist.ChannelID, playlist.UploaderID),
	}
}

func infoJSONOutputPath(outputTemplate string, info *client.VideoInfo, restrictFilenamesAndTrim ...any) string {
	restrict, trimLimit := templatePathOptions(restrictFilenamesAndTrim...)
	return client.InfoJSONOutputPath(info, client.SidecarPathOptions{
		OutputTemplate:    outputTemplate,
		RestrictFilenames: restrict,
		TrimFilenames:     trimLimit,
	})
}

func writeDescriptionSidecar(info *client.VideoInfo, opts cli.Options) error {
	outputPath := descriptionOutputPath(effectiveOutputTemplate(opts), info, opts.RestrictFilenames, opts.TrimFilenames)
	if shouldSkipExistingOutput(outputPath, opts) {
		if shouldPrintHumanText(opts) {
			fmt.Printf("Skipping existing description: %s\n", outputPath)
		}
		return nil
	}
	if err := client.WriteDescriptionSidecar(info, outputPath); err != nil {
		return err
	}
	if shouldPrintHumanText(opts) {
		fmt.Printf("Written description: %s\n", outputPath)
	}
	return nil
}

type shortcutKind string

const (
	shortcutURL     shortcutKind = "url"
	shortcutWebloc  shortcutKind = "webloc"
	shortcutDesktop shortcutKind = "desktop"
)

func writeURLLinkSidecar(input string, info *client.VideoInfo, opts cli.Options) error {
	return writeShortcutSidecar(input, info, opts, shortcutURL)
}

func writeShortcutSidecar(input string, info *client.VideoInfo, opts cli.Options, kind shortcutKind) error {
	outputPath := shortcutOutputPath(effectiveOutputTemplate(opts), info, kind, opts.RestrictFilenames, opts.TrimFilenames)
	if shouldSkipExistingOutput(outputPath, opts) {
		if shouldPrintHumanText(opts) {
			fmt.Printf("Skipping existing %s link: %s\n", kind, outputPath)
		}
		return nil
	}
	if err := client.WriteShortcutSidecar(input, info, client.ShortcutKind(kind), outputPath); err != nil {
		return err
	}
	if shouldPrintHumanText(opts) {
		fmt.Printf("Written %s link: %s\n", kind, outputPath)
	}
	return nil
}

func urlLinkOutputPath(outputTemplate string, info *client.VideoInfo, restrictFilenamesAndTrim ...any) string {
	return shortcutOutputPath(outputTemplate, info, shortcutURL, restrictFilenamesAndTrim...)
}

func shortcutOutputPath(outputTemplate string, info *client.VideoInfo, kind shortcutKind, restrictFilenamesAndTrim ...any) string {
	restrict, trimLimit := templatePathOptions(restrictFilenamesAndTrim...)
	return client.ShortcutOutputPath(info, client.ShortcutKind(kind), client.SidecarPathOptions{
		OutputTemplate:    outputTemplate,
		RestrictFilenames: restrict,
		TrimFilenames:     trimLimit,
	})
}

func shortcutSidecarBody(input string, info *client.VideoInfo, kind shortcutKind) string {
	return client.ShortcutSidecarBody(input, info, client.ShortcutKind(kind))
}

func descriptionOutputPath(outputTemplate string, info *client.VideoInfo, restrictFilenamesAndTrim ...any) string {
	restrict, trimLimit := templatePathOptions(restrictFilenamesAndTrim...)
	return client.DescriptionOutputPath(info, client.SidecarPathOptions{
		OutputTemplate:    outputTemplate,
		RestrictFilenames: restrict,
		TrimFilenames:     trimLimit,
	})
}

func writeThumbnailSidecar(ctx context.Context, c *client.Client, info *client.VideoInfo, opts cli.Options) error {
	outputPath := thumbnailOutputPath(effectiveOutputTemplate(opts), info, opts.RestrictFilenames, opts.TrimFilenames)
	if shouldSkipExistingOutput(outputPath, opts) {
		if shouldPrintHumanText(opts) {
			fmt.Printf("Skipping existing thumbnail: %s\n", outputPath)
		}
		return nil
	}
	if err := c.DownloadThumbnail(ctx, info, outputPath); err != nil {
		return err
	}
	if shouldPrintHumanText(opts) {
		fmt.Printf("Written thumbnail: %s\n", outputPath)
	}
	return nil
}

func thumbnailOutputPath(outputTemplate string, info *client.VideoInfo, restrictFilenamesAndTrim ...any) string {
	restrict, trimLimit := templatePathOptions(restrictFilenamesAndTrim...)
	return client.ThumbnailOutputPath(info, client.SidecarPathOptions{
		OutputTemplate:    outputTemplate,
		RestrictFilenames: restrict,
		TrimFilenames:     trimLimit,
	})
}

func thumbnailExt(rawURL string) string {
	return client.ThumbnailExt(rawURL)
}

func sanitizeTemplateToken(v string) string {
	return client.SanitizeOutputTemplateToken(v)
}

func sanitizeRestrictedTemplateToken(v string) string {
	return client.SanitizeRestrictedOutputTemplateToken(v)
}

func shouldSkipDownloadByArchive(input string, opts cli.Options) bool {
	if activeDownloadArchive == nil {
		return false
	}
	videoID, err := client.ExtractVideoID(input)
	if err != nil {
		return false
	}
	if !activeDownloadArchive.Has(videoID) {
		return false
	}
	if shouldPrintHumanText(opts) {
		fmt.Printf("Skipping (in archive): %s\n", videoID)
	}
	return true
}

func recordCompletedDownload(videoID string) error {
	if activeDownloadArchive == nil {
		return incrementDownloadLimit()
	}
	if err := activeDownloadArchive.Add(videoID); err != nil {
		return fmt.Errorf("failed to update download archive: %w", err)
	}
	return incrementDownloadLimit()
}

func recordForcedArchiveIfRequested(info *client.VideoInfo, opts cli.Options) error {
	if !opts.ForceWriteArchive || info == nil {
		return nil
	}
	return recordCompletedDownload(info.ID)
}

type downloadLimit struct {
	Max   int
	Count int
}

func incrementDownloadLimit() error {
	if activeDownloadLimit == nil || activeDownloadLimit.Max <= 0 {
		return nil
	}
	activeDownloadLimit.Count++
	if activeDownloadLimit.Count >= activeDownloadLimit.Max {
		return errMaxDownloadsReached
	}
	return nil
}

func printAttemptDiagnostics(err error) {
	attempts, ok := client.AttemptDetails(err)
	if !ok || len(attempts) == 0 {
		printGenericRemediationHints(err)
		return
	}
	fmt.Println("Attempt diagnostics:")
	for i, a := range attempts {
		fmt.Printf("  [%d] client=%s stage=%s", i+1, a.Client, a.Stage)
		if a.Itag != 0 {
			fmt.Printf(" itag=%d", a.Itag)
		}
		if a.Protocol != "" {
			fmt.Printf(" proto=%s", a.Protocol)
		}
		if a.HTTPStatus != 0 {
			fmt.Printf(" http=%d", a.HTTPStatus)
		}
		if a.URLHost != "" {
			fmt.Printf(" host=%s", a.URLHost)
		}
		if a.URLHasN {
			fmt.Printf(" has_n=true")
		}
		if a.URLHasPOT {
			fmt.Printf(" has_pot=true")
		}
		if a.URLHasSignature {
			fmt.Printf(" has_sig=true")
		}
		if a.POTRequired {
			fmt.Printf(" pot_required=true")
		}
		if a.Reason != "" {
			fmt.Printf(" reason=%q", a.Reason)
		}
		fmt.Println()
	}
	for _, hint := range remediationHintsForAttempts(attempts) {
		fmt.Println(hint)
	}
}

func printGenericRemediationHints(err error) {
	var noPlayableDetail *client.NoPlayableFormatsDetailError
	switch {
	case errors.Is(err, client.ErrInvalidInput):
		fmt.Println("hint: unsupported input. Use a full YouTube URL or 11-char video ID, then retry.")
	case errors.Is(err, client.ErrLoginRequired):
		fmt.Println("hint: login-required content. Retry with --cookies <netscape.txt> and --visitor-data <VISITOR_INFO1_LIVE>.")
	case errors.Is(err, client.ErrNoPlayableFormats):
		if errors.As(err, &noPlayableDetail) && noPlayableDetail.Selector != "" {
			fmt.Printf("hint: selector %q matched no formats (%s). Retry with -F and adjust -f expression.\n", noPlayableDetail.Selector, noPlayableDetail.SelectionError)
			return
		}
		fmt.Println("hint: no playable formats. Retry with -F to inspect candidates and --verbose for extraction stages.")
	case errors.Is(err, client.ErrChallengeNotSolved):
		fmt.Println("hint: challenge solve failed. Retry with --verbose and inspect [extract] challenge:* logs.")
	case errors.Is(err, client.ErrMP3TranscoderNotConfigured):
		fmt.Println("hint: mp3 mode requires an MP3 transcoder. Configure client.Config.MP3Transcoder (CLI: use a build with transcoder wiring).")
	default:
		fmt.Println("hint: retry with --verbose --override-diagnostics to inspect stage/client failure details.")
	}
}

func remediationHintsForAttempts(attempts []client.AttemptDetail) []string {
	var hints []string
	sawLogin := false
	sawPOTRequired := false
	sawMissingPOT := false
	sawNoN := false
	sawHTTP403 := false
	sawHTTP429 := false

	for _, a := range attempts {
		if a.LoginRequired {
			sawLogin = true
		}
		if a.POTRequired {
			sawPOTRequired = true
			if !a.POTAvailable {
				sawMissingPOT = true
			}
		}
		if !a.URLHasN {
			sawNoN = true
		}
		if a.HTTPStatus == 403 {
			sawHTTP403 = true
		}
		if a.HTTPStatus == 429 {
			sawHTTP429 = true
		}
	}

	if sawLogin {
		hints = append(hints, "hint: login-required restriction detected. Retry with --cookies <netscape.txt> and, if needed, --visitor-data <VISITOR_INFO1_LIVE>.")
	}
	if sawPOTRequired && sawMissingPOT {
		hints = append(hints, "hint: missing required POT detected. Supply --po-token <token> or configure client.Config.PoTokenProvider.")
	}
	if sawHTTP429 {
		hints = append(hints, "hint: upstream throttling (HTTP 429). Retry later or use lower-concurrency network settings.")
	}
	if sawHTTP403 && sawNoN {
		hints = append(hints, "hint: 403 + missing n-signature observed. Retry with --verbose and verify [extract] challenge:success logs.")
	}
	if len(hints) == 0 {
		hints = append(hints, "hint: retry with --verbose --override-diagnostics to inspect client/stage-specific failure details.")
	}
	return hints
}

func formatExtractionEvent(evt client.ExtractionEvent) string {
	scope := evt.Stage + ":" + evt.Phase
	if evt.Client != "" {
		scope += " client=" + evt.Client
	}
	if evt.Detail != "" {
		scope += " detail=" + evt.Detail
	}
	return "[extract] " + scope
}

func formatDownloadEvent(evt client.DownloadEvent) string {
	scope := evt.Stage + ":" + evt.Phase
	if evt.VideoID != "" {
		scope += " video_id=" + evt.VideoID
	}
	if evt.Path != "" {
		scope += " path=" + evt.Path
	}
	if evt.Detail != "" {
		scope += " detail=" + evt.Detail
	}
	return "[download] " + scope
}

type lifecyclePrinter struct {
	now func() time.Time
	mu  sync.Mutex
	// key: stage|client
	extractStarts map[string]time.Time
	// key: stage|videoID|path
	downloadStarts map[string]time.Time
	// key: videoID
	videoTimings map[string]videoTiming
}

func newLifecyclePrinter(now func() time.Time) *lifecyclePrinter {
	return &lifecyclePrinter{
		now:            now,
		extractStarts:  make(map[string]time.Time),
		downloadStarts: make(map[string]time.Time),
		videoTimings:   make(map[string]videoTiming),
	}
}

type videoTiming struct {
	downloadVideoMs  int64
	downloadAudioMs  int64
	downloadSingleMs int64
	mergeMs          int64
}

func (p *lifecyclePrinter) formatExtractionEvent(evt client.ExtractionEvent) string {
	detail := evt.Detail
	key := evt.Stage + "|" + evt.Client

	p.mu.Lock()
	switch evt.Phase {
	case "start":
		p.extractStarts[key] = p.now()
	case "success", "failure", "partial", "complete":
		if started, ok := p.extractStarts[key]; ok {
			detail = appendDetail(detail, fmt.Sprintf("elapsed_ms=%d", p.now().Sub(started).Milliseconds()))
			delete(p.extractStarts, key)
		}
	}
	p.mu.Unlock()

	return formatExtractionEvent(client.ExtractionEvent{
		Stage:  evt.Stage,
		Phase:  evt.Phase,
		Client: evt.Client,
		Detail: detail,
	})
}

func (p *lifecyclePrinter) formatDownloadEvent(evt client.DownloadEvent) string {
	detail := evt.Detail
	key := evt.Stage + "|" + evt.VideoID + "|" + evt.Path
	now := p.now()

	p.mu.Lock()
	switch evt.Phase {
	case "start", "delete":
		p.downloadStarts[key] = now
	case "complete", "failure", "skip":
		if started, ok := p.downloadStarts[key]; ok {
			elapsed := now.Sub(started).Milliseconds()
			detail = appendDetail(detail, fmt.Sprintf("elapsed_ms=%d", elapsed))
			if evt.Stage == "download" && evt.Phase == "complete" {
				if bytes, ok := extractBytesFromDetail(detail); ok && elapsed > 0 {
					seconds := float64(elapsed) / 1000.0
					speedBPS := float64(bytes) / seconds
					speedMiB := speedBPS / (1024.0 * 1024.0)
					detail = appendDetail(detail, fmt.Sprintf("speed_bps=%d", int64(speedBPS)))
					detail = appendDetail(detail, fmt.Sprintf("speed_mib_s=%.2f", speedMiB))
				}
				role := inferDownloadRole(evt.Path)
				detail = appendDetail(detail, "part="+role)
				vt := p.videoTimings[evt.VideoID]
				switch role {
				case "video":
					vt.downloadVideoMs += elapsed
				case "audio":
					vt.downloadAudioMs += elapsed
				default:
					vt.downloadSingleMs += elapsed
				}
				p.videoTimings[evt.VideoID] = vt
			}
			if evt.Stage == "merge" && evt.Phase == "complete" {
				vt := p.videoTimings[evt.VideoID]
				vt.mergeMs += elapsed
				p.videoTimings[evt.VideoID] = vt
			}
			delete(p.downloadStarts, key)
		}
	}
	p.mu.Unlock()

	return formatDownloadEvent(client.DownloadEvent{
		Stage:   evt.Stage,
		Phase:   evt.Phase,
		VideoID: evt.VideoID,
		Path:    evt.Path,
		Detail:  detail,
	})
}

func (p *lifecyclePrinter) popVideoTiming(videoID string) videoTiming {
	p.mu.Lock()
	defer p.mu.Unlock()
	vt := p.videoTimings[videoID]
	delete(p.videoTimings, videoID)
	return vt
}

func appendDetail(base string, extra string) string {
	extra = strings.TrimSpace(extra)
	if extra == "" {
		return base
	}
	base = strings.TrimSpace(base)
	if base == "" {
		return extra
	}
	if strings.Contains(base, extra) {
		return base
	}
	return base + " " + extra
}

func extractBytesFromDetail(detail string) (int64, bool) {
	tokens := strings.FieldsFunc(detail, func(r rune) bool {
		return r == ' ' || r == ','
	})
	for _, token := range tokens {
		if !strings.HasPrefix(token, "bytes=") {
			continue
		}
		raw := strings.TrimPrefix(token, "bytes=")
		v, err := strconv.ParseInt(raw, 10, 64)
		if err == nil && v >= 0 {
			return v, true
		}
	}
	return 0, false
}

func inferDownloadRole(path string) string {
	lower := strings.ToLower(strings.TrimSpace(path))
	switch {
	case strings.HasSuffix(lower, ".video"):
		return "video"
	case strings.HasSuffix(lower, ".audio"):
		return "audio"
	default:
		return "single"
	}
}

type ytdlpDumpSingleJSON = client.YTDLPDumpSingleJSON
type ytdlpPlaylistInfoJSON = client.YTDLPPlaylistInfoJSON
type ytdlpPlaylistItemEntry = client.YTDLPPlaylistItemEntry
type ytdlpFormatEntry = client.YTDLPFormatEntry

func emitDumpSingleJSON(w io.Writer, input string, info *client.VideoInfo) error {
	payload := buildDumpSingleJSONPayload(input, info)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

func emitPlaylistInfoJSON(w io.Writer, playlist *client.PlaylistInfo) error {
	payload := buildPlaylistInfoJSONPayload(playlist)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

func buildPlaylistInfoJSONPayload(playlist *client.PlaylistInfo) ytdlpPlaylistInfoJSON {
	return client.BuildYTDLPPlaylistInfoJSON(playlist)
}

func buildDumpSingleJSONPayload(input string, info *client.VideoInfo) ytdlpDumpSingleJSON {
	return client.BuildYTDLPDumpSingleJSON(input, info)
}

func canonicalWatchURL(input string, videoID string) string {
	return client.CanonicalWatchURL(input, videoID)
}

func canonicalPlaylistURL(playlistID string) string {
	return client.CanonicalPlaylistURL(playlistID)
}

func pickBestDirectFormatURL(formats []client.FormatInfo) (string, string) {
	return client.PickBestDirectFormatURL(formats)
}

func compareFormatQuality(a, b client.FormatInfo) int {
	return client.CompareFormatQuality(a, b)
}

type cliErrorReport struct {
	OK       bool           `json:"ok"`
	Input    string         `json:"input"`
	ExitCode int            `json:"exit_code"`
	Error    cliErrorDetail `json:"error"`
}

type cliErrorDetail struct {
	Category string                 `json:"category"`
	Message  string                 `json:"message"`
	Attempts []client.AttemptDetail `json:"attempts,omitempty"`
}

func emitJSONFailure(input string, err error, exitCode int) {
	report := cliErrorReport{
		OK:       false,
		Input:    input,
		ExitCode: exitCode,
		Error: cliErrorDetail{
			Category: string(client.ClassifyError(err)),
			Message:  err.Error(),
		},
	}
	if attempts, ok := client.AttemptDetails(err); ok && len(attempts) > 0 {
		report.Error.Attempts = attempts
	}
	_ = json.NewEncoder(os.Stdout).Encode(report)
}

func classifyExitCode(err error) int {
	switch client.ClassifyError(err) {
	case client.ErrorCategoryInvalidInput:
		return exitCodeInvalidInput
	case client.ErrorCategoryLoginRequired:
		return exitCodeLoginRequired
	case client.ErrorCategoryUnavailable:
		return exitCodeUnavailable
	case client.ErrorCategoryNoPlayableFormats:
		return exitCodeNoPlayableFormats
	case client.ErrorCategoryChallengeNotSolved:
		return exitCodeChallengeUnresolved
	case client.ErrorCategoryAllClientsFailed:
		return exitCodeAllClientsFailed
	case client.ErrorCategoryDownloadFailed:
		return exitCodeDownloadFailed
	case client.ErrorCategoryMP3TranscoderNotConfigured:
		return exitCodeMP3ConfigRequired
	case client.ErrorCategoryTranscriptParse:
		return exitCodeTranscriptParse
	default:
		return exitCodeGenericFailure
	}
}

func newDownloadArchive(path string) (*client.DownloadArchive, error) {
	return client.OpenDownloadArchive(path)
}
