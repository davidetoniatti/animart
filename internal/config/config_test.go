package config

import (
	"io"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestParseDefaults(t *testing.T) {
	music := t.TempDir()

	cfg, err := parse([]string{"-music", music}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MusicPath != music || cfg.CoverFileName != "cover.webp" || cfg.Storefront != "us" ||
		cfg.Quality != 50 || cfg.Scale != 800 || cfg.FPS != 20 ||
		cfg.Delay != 2*time.Second || cfg.Interval != 24*time.Hour || cfg.NegativeTTL != 720*time.Hour ||
		cfg.Excludes != nil || cfg.Once || cfg.Force || cfg.Verbose {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestParseFlags(t *testing.T) {
	music := t.TempDir()

	cfg, err := parse([]string{
		"-music", music, "-exclude", " explo, ,Discover ", "-delay-ms", "500", "-interval", "0",
		"-storefront", "it", "-once", "-force",
	}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"explo", "Discover"}; !reflect.DeepEqual(cfg.Excludes, want) {
		t.Errorf("Excludes = %q, want %q", cfg.Excludes, want)
	}
	if cfg.Delay != 500*time.Millisecond || cfg.Interval != 0 || cfg.Storefront != "it" || !cfg.Once || !cfg.Force {
		t.Errorf("unexpected config: %+v", cfg)
	}
}

func TestParseInvalid(t *testing.T) {
	music := t.TempDir()

	tests := map[string][]string{
		"missing music":       {},
		"music not found":     {"-music", filepath.Join(music, "missing")},
		"cover name with dir": {"-music", music, "-cover-name", "../cover.webp"},
		"empty cover name":    {"-music", music, "-cover-name", ""},
		"storefront":          {"-music", music, "-storefront", "USA"},
		"quality":             {"-music", music, "-quality", "101"},
		"scale":               {"-music", music, "-scale", "0"},
		"fps":                 {"-music", music, "-fps", "-1"},
		"delay":               {"-music", music, "-delay-ms", "-1"},
		"negative ttl":        {"-music", music, "-negative-ttl", "-1h"},
		"interval":            {"-music", music, "-interval", "daily"},
		"force without once":  {"-music", music, "-force"},
		"unknown flag":        {"-music", music, "-bogus"},
	}
	for name, args := range tests {
		if _, err := parse(args, io.Discard); err == nil {
			t.Errorf("%s: expected an error for %q", name, args)
		}
	}
}
