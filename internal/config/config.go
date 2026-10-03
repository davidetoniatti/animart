package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	MusicPath     string
	Excludes      []string
	CoverFileName string
	CachePath     string
	Delay         time.Duration
	NegativeTTL   time.Duration
	Quality       int
	Scale         int
	FPS           int
	Interval      time.Duration
	Once          bool
	Verbose       bool
}

func Parse() *Config {
	cfg := &Config{}

	var excludesStr string
	var delayMs int
	var intervalStr string

	defaultCache := "cache.db"
	if dir, err := os.UserCacheDir(); err == nil {
		defaultCache = filepath.Join(dir, "animart", "cache.db")
	}

	flag.StringVar(&cfg.MusicPath, "music", "", "Path to music library root (required)")
	flag.StringVar(&excludesStr, "exclude", "", "Comma-separated folder names to exclude from scan")
	flag.StringVar(&cfg.CoverFileName, "cover-name", "cover.webp", "Output filename for animated covers")
	flag.StringVar(&cfg.CachePath, "cache", defaultCache, "Path to SQLite cache database")
	flag.IntVar(&delayMs, "delay-ms", 2000, "Delay in milliseconds between Apple Music requests")
	flag.DurationVar(&cfg.NegativeTTL, "negative-ttl", 30*24*time.Hour, "How long albums without animated artwork are skipped before a new lookup (0 = forever)")
	flag.IntVar(&cfg.Quality, "quality", 50, "WebP encoding quality (0-100)")
	flag.IntVar(&cfg.Scale, "scale", 800, "Max resolution width/height in pixels")
	flag.IntVar(&cfg.FPS, "fps", 20, "Target frame rate (FPS) for WebP animation")
	flag.StringVar(&intervalStr, "interval", "24h", "Rescan interval (e.g. 24h, 12h, 0 to disable)")
	flag.BoolVar(&cfg.Once, "once", false, "Run once and exit (do not keep running as daemon)")
	flag.BoolVar(&cfg.Verbose, "verbose", false, "Enable verbose debug logs")

	flag.Parse()

	if cfg.MusicPath == "" {
		fmt.Fprintln(os.Stderr, "error: -music is required")
		flag.Usage()
		os.Exit(2)
	}
	if info, err := os.Stat(cfg.MusicPath); err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "error: music library %q is not an accessible directory\n", cfg.MusicPath)
		os.Exit(2)
	}

	// Parse comma-separated excludes
	for _, part := range strings.Split(excludesStr, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			cfg.Excludes = append(cfg.Excludes, trimmed)
		}
	}

	cfg.Delay = time.Duration(delayMs) * time.Millisecond

	if interval, err := time.ParseDuration(intervalStr); err == nil {
		cfg.Interval = interval
	} else {
		cfg.Interval = 24 * time.Hour
	}

	return cfg
}

func (c *Config) IsExcluded(name string) bool {
	for _, excl := range c.Excludes {
		if strings.EqualFold(excl, name) {
			return true
		}
	}
	return false
}
