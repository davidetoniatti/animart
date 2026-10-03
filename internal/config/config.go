package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var storefrontRegex = regexp.MustCompile(`^[a-z]{2}$`)

type Config struct {
	MusicPath     string
	Excludes      []string
	CoverFileName string
	CachePath     string
	Delay         time.Duration
	NegativeTTL   time.Duration
	Storefront    string
	Quality       int
	Scale         int
	FPS           int
	Interval      time.Duration
	Once          bool
	Force         bool
	Verbose       bool
	Version       bool
}

// Parse reads the command line and exits with status 2 on invalid flags.
func Parse() *Config {
	cfg, err := parse(os.Args[1:], os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		os.Exit(0)
	}
	if err != nil {
		os.Exit(2)
	}
	return cfg
}

// parse parses args and validates them, writing usage and errors to output.
func parse(args []string, output io.Writer) (*Config, error) {
	cfg := &Config{}
	var excludes string
	var delayMs int

	defaultCache := "cache.db"
	if dir, err := os.UserCacheDir(); err == nil {
		defaultCache = filepath.Join(dir, "animart", "cache.db")
	}

	fs := flag.NewFlagSet("animart", flag.ContinueOnError)
	fs.SetOutput(output)
	fs.StringVar(&cfg.MusicPath, "music", "", "Path to music library root (required)")
	fs.StringVar(&excludes, "exclude", "", "Comma-separated folder names to exclude from scan")
	fs.StringVar(&cfg.CoverFileName, "cover-name", "cover.webp", "Output filename for animated covers")
	fs.StringVar(&cfg.CachePath, "cache", defaultCache, "Path to SQLite cache database")
	fs.IntVar(&delayMs, "delay-ms", 2000, "Delay in milliseconds between Apple Music requests")
	fs.StringVar(&cfg.Storefront, "storefront", "us", "Apple Music storefront (two-letter country code) used for searches")
	fs.DurationVar(&cfg.NegativeTTL, "negative-ttl", 30*24*time.Hour, "How long albums without animated artwork are skipped before a new lookup (0 = forever)")
	fs.IntVar(&cfg.Quality, "quality", 50, "WebP encoding quality (0-100)")
	fs.IntVar(&cfg.Scale, "scale", 800, "Max resolution width/height in pixels")
	fs.IntVar(&cfg.FPS, "fps", 20, "Target frame rate (FPS) for WebP animation")
	fs.DurationVar(&cfg.Interval, "interval", 24*time.Hour, "Rescan interval (e.g. 24h, 12h, 0 to run once)")
	fs.BoolVar(&cfg.Once, "once", false, "Run once and exit (do not keep running as daemon)")
	fs.BoolVar(&cfg.Force, "force", false, "Replace existing covers (requires -once)")
	fs.BoolVar(&cfg.Verbose, "verbose", false, "Enable verbose debug logs")
	fs.BoolVar(&cfg.Version, "version", false, "Print the version and exit")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if cfg.Version {
		return cfg, nil
	}

	for _, name := range strings.Split(excludes, ",") {
		if name = strings.TrimSpace(name); name != "" {
			cfg.Excludes = append(cfg.Excludes, name)
		}
	}
	cfg.Delay = time.Duration(delayMs) * time.Millisecond

	if err := cfg.validate(); err != nil {
		fmt.Fprintln(output, "error:", err)
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	switch {
	case c.MusicPath == "":
		return errors.New("-music is required")
	case c.CoverFileName != filepath.Base(c.CoverFileName) || c.CoverFileName == "." || c.CoverFileName == "..":
		return fmt.Errorf("-cover-name must be a plain file name, got %q", c.CoverFileName)
	case !storefrontRegex.MatchString(c.Storefront):
		return fmt.Errorf("-storefront must be a two-letter lowercase country code, got %q", c.Storefront)
	case c.Quality < 0 || c.Quality > 100:
		return fmt.Errorf("-quality must be between 0 and 100, got %d", c.Quality)
	case c.Scale <= 0:
		return fmt.Errorf("-scale must be positive, got %d", c.Scale)
	case c.FPS <= 0:
		return fmt.Errorf("-fps must be positive, got %d", c.FPS)
	case c.Delay < 0:
		return errors.New("-delay-ms must not be negative")
	case c.NegativeTTL < 0:
		return errors.New("-negative-ttl must not be negative")
	case c.Interval < 0:
		return errors.New("-interval must not be negative")
	case c.Force && !c.Once:
		return errors.New("-force requires -once")
	}

	if info, err := os.Stat(c.MusicPath); err != nil || !info.IsDir() {
		return fmt.Errorf("music library %q is not an accessible directory", c.MusicPath)
	}
	return nil
}
