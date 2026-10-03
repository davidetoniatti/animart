package cache

import (
	"path/filepath"
	"testing"
	"time"
)

func TestCacheRoundTrip(t *testing.T) {
	c, err := Open(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if e, err := c.Get("Korn", "Issues"); err != nil || e != nil {
		t.Fatalf("got (%+v, %v), want (nil, nil)", e, err)
	}

	start := time.Now().Add(-time.Second)
	if err := c.SetNegative("Korn", "Issues"); err != nil {
		t.Fatal(err)
	}
	e, err := c.Get(" korn ", "ISSUES")
	if err != nil || e == nil || e.HasArtwork || e.UpdatedAt.Before(start) {
		t.Fatalf("got (%+v, %v), want a fresh negative entry", e, err)
	}

	if err := c.SetPositive("Korn", "Issues", "https://example.com/v.m3u8"); err != nil {
		t.Fatal(err)
	}
	e, err = c.Get("Korn", "Issues")
	if err != nil || e == nil || !e.HasArtwork || e.VideoURL != "https://example.com/v.m3u8" {
		t.Fatalf("got (%+v, %v), want a positive entry", e, err)
	}
}
