package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/davidetoniatti/animart/internal/applemusic"
	"github.com/davidetoniatti/animart/internal/cache"
	"github.com/davidetoniatti/animart/internal/config"
	"github.com/davidetoniatti/animart/internal/converter"
	"github.com/davidetoniatti/animart/internal/scanner"
)

const maxRateLimitRetries = 5

func main() {
	cfg := config.Parse()

	log.Printf("[AnimArt] Starting Animated Artwork Downloader in Go")
	log.Printf("[AnimArt] Music Library: %s", cfg.MusicPath)
	log.Printf("[AnimArt] Excluded Folders: %v", cfg.Excludes)
	log.Printf("[AnimArt] Output Cover Name: %s", cfg.CoverFileName)
	log.Printf("[AnimArt] Cache DB: %s", cfg.CachePath)
	log.Printf("[AnimArt] Target Format: WebP (Scale: %dpx, FPS: %d, Quality: %d)", cfg.Scale, cfg.FPS, cfg.Quality)

	db, err := cache.Open(cfg.CachePath)
	if err != nil {
		log.Fatalf("[AnimArt] Failed to open cache: %v", err)
	}
	defer db.Close()

	conv, err := converter.NewConverter()
	if err != nil {
		log.Fatalf("[AnimArt] Failed to initialize converter: %v", err)
	}

	amClient := applemusic.NewClient()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Printf("[AnimArt] Shutdown signal received, terminating gracefully...")
		cancel()
	}()

	runSync(ctx, cfg, db, amClient, conv)

	if cfg.Once {
		log.Printf("[AnimArt] Single run completed. Exiting.")
		return
	}

	if cfg.Interval <= 0 {
		return
	}

	log.Printf("[AnimArt] Scheduled periodic scans every %v", cfg.Interval)
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runSync(ctx, cfg, db, amClient, conv)
		}
	}
}

func runSync(ctx context.Context, cfg *config.Config, db *cache.Cache, amClient *applemusic.Client, conv *converter.Converter) {
	log.Printf("[AnimArt] Starting music library scan...")
	scanStart := time.Now()

	albums, err := scanner.Scan(cfg.MusicPath, cfg.Excludes, cfg.CoverFileName)
	if err != nil {
		log.Printf("[AnimArt] Scan error: %v", err)
		return
	}

	log.Printf("[AnimArt] Scan complete in %v. Found %d albums needing animated artwork.", time.Since(scanStart), len(albums))
	if len(albums) == 0 {
		return
	}

	downloadedCount := 0
	skippedNegative := 0

	for i, alb := range albums {
		select {
		case <-ctx.Done():
			log.Printf("[AnimArt] Sync interrupted.")
			return
		default:
		}

		progress := fmt.Sprintf("[%d/%d]", i+1, len(albums))
		coverPath := filepath.Join(alb.Path, cfg.CoverFileName)

		// Double check if file exists
		if _, err := os.Stat(coverPath); err == nil {
			continue
		}

		entry, err := db.Get(alb.Artist, alb.Album)
		if err != nil {
			log.Printf("[AnimArt] %s Cache read error for %s - %s: %v", progress, alb.Artist, alb.Album, err)
		}

		knownMissing := entry != nil && !entry.HasArtwork &&
			(cfg.NegativeTTL <= 0 || time.Since(entry.UpdatedAt) < cfg.NegativeTTL)
		if knownMissing {
			if cfg.Verbose {
				log.Printf("[AnimArt] %s Skipping %s - %s (cached negative)", progress, alb.Artist, alb.Album)
			}
			skippedNegative++
			continue
		}

		var videoURL string
		if entry != nil && entry.HasArtwork && entry.VideoURL != "" {
			videoURL = entry.VideoURL
			log.Printf("[AnimArt] %s Found in positive cache: %s - %s", progress, alb.Artist, alb.Album)
		} else {
			log.Printf("[AnimArt] %s Searching Apple Music: %s - %s", progress, alb.Artist, alb.Album)
			match, err := searchWithBackoff(ctx, amClient, alb.Artist, alb.Album)
			if errors.Is(err, applemusic.ErrRateLimited) {
				log.Printf("[AnimArt] Apple Music is still rate limiting, ending sync cycle.")
				return
			}
			if err != nil {
				// Not cached: the album is looked up again on the next scan.
				log.Printf("[AnimArt] %s Search error for %s - %s: %v", progress, alb.Artist, alb.Album, err)
				sleep(ctx, cfg.Delay)
				continue
			}

			if match == nil {
				log.Printf("[AnimArt] %s No animated cover available for %s - %s", progress, alb.Artist, alb.Album)
				_ = db.SetNegative(alb.Artist, alb.Album)
				sleep(ctx, cfg.Delay)
				continue
			}

			log.Printf("[AnimArt] %s Matched Apple Music album: %s - %s (id %s)", progress, match.Artist, match.Album, match.ID)
			videoURL = match.VideoURL
			_ = db.SetPositive(alb.Artist, alb.Album, videoURL)
			sleep(ctx, cfg.Delay)
		}

		// Download and convert
		log.Printf("[AnimArt] %s Converting animated cover to %s ...", progress, coverPath)
		convStart := time.Now()
		err = conv.ConvertHlsToWebp(ctx, videoURL, coverPath, cfg.Scale, cfg.FPS, cfg.Quality)
		if err != nil {
			log.Printf("[AnimArt] %s Conversion error: %v", progress, err)
			continue
		}

		info, err := os.Stat(coverPath)
		var sizeMB float64
		if err == nil {
			sizeMB = float64(info.Size()) / (1024 * 1024)
		}

		log.Printf("[AnimArt] %s Saved %s (%.2f MB) in %v", progress, cfg.CoverFileName, sizeMB, time.Since(convStart))
		downloadedCount++
	}

	log.Printf("[AnimArt] Sync cycle finished: %d new covers downloaded, %d skipped from negative cache.", downloadedCount, skippedNegative)
}

// searchWithBackoff retries rate-limited searches, waiting one minute and
// doubling the wait on every retry.
func searchWithBackoff(ctx context.Context, c *applemusic.Client, artist, album string) (*applemusic.Match, error) {
	wait := time.Minute
	for retry := 0; ; retry++ {
		match, err := c.SearchAnimatedCover(ctx, artist, album)
		if !errors.Is(err, applemusic.ErrRateLimited) || retry == maxRateLimitRetries {
			return match, err
		}
		log.Printf("[AnimArt] Rate limited by Apple Music, retrying in %v", wait)
		if err := sleep(ctx, wait); err != nil {
			return nil, err
		}
		wait *= 2
	}
}

// sleep waits for d or until ctx is cancelled.
func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
