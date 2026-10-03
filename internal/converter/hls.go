package converter

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var (
	resolutionRegex = regexp.MustCompile(`RESOLUTION=(\d+)x(\d+)`)
	bandwidthRegex  = regexp.MustCompile(`[:,]BANDWIDTH=(\d+)`)
	codecsRegex     = regexp.MustCompile(`CODECS="([^"]*)"`)
)

type variant struct {
	url       string
	width     int
	height    int
	bandwidth int
	avc       bool
}

func (v variant) minSide() int { return min(v.width, v.height) }
func (v variant) area() int    { return v.width * v.height }

// resolveVariant returns the variant of an HLS master playlist that best fits
// an output scaled to scale pixels per side. Other URLs are returned unchanged.
func (c *Converter) resolveVariant(ctx context.Context, playlistURL string, scale int) (string, error) {
	base, err := url.Parse(playlistURL)
	if err != nil {
		return "", err
	}
	if !strings.HasSuffix(base.Path, ".m3u8") {
		return playlistURL, nil
	}

	req, err := http.NewRequestWithContext(ctx, "GET", playlistURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("playlist status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}

	variants := parseVariants(base, string(body))
	if len(variants) == 0 {
		return playlistURL, nil
	}
	return pickVariant(variants, scale).url, nil
}

// parseVariants returns the video variants listed in an HLS master playlist.
func parseVariants(base *url.URL, playlist string) []variant {
	var variants []variant
	var pending *variant
	for _, line := range strings.Split(playlist, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "#EXT-X-STREAM-INF:"):
			pending = parseStreamInf(line)
		case pending != nil && line != "" && !strings.HasPrefix(line, "#"):
			if ref, err := base.Parse(line); err == nil {
				pending.url = ref.String()
				variants = append(variants, *pending)
			}
			pending = nil
		}
	}
	return variants
}

func parseStreamInf(line string) *variant {
	v := &variant{}
	if m := resolutionRegex.FindStringSubmatch(line); m != nil {
		v.width, _ = strconv.Atoi(m[1])
		v.height, _ = strconv.Atoi(m[2])
	}
	if m := bandwidthRegex.FindStringSubmatch(line); m != nil {
		v.bandwidth, _ = strconv.Atoi(m[1])
	}
	if m := codecsRegex.FindStringSubmatch(line); m != nil {
		v.avc = strings.HasPrefix(m[1], "avc1")
	}
	return v
}

// pickVariant returns the smallest variant whose shorter side is at least
// minSide, or the largest variant when none is big enough. H.264 wins over
// other codecs, lower bandwidth breaks the remaining ties.
func pickVariant(variants []variant, minSide int) variant {
	best := variants[0]
	for _, v := range variants[1:] {
		if better(v, best, minSide) {
			best = v
		}
	}
	return best
}

func better(a, b variant, minSide int) bool {
	aBig, bBig := a.minSide() >= minSide, b.minSide() >= minSide
	switch {
	case aBig != bBig:
		return aBig
	case aBig && a.avc != b.avc:
		return a.avc
	case a.area() != b.area():
		// Big enough: the smaller wins. Too small: the larger wins.
		return (a.area() < b.area()) == aBig
	case a.avc != b.avc:
		return a.avc
	default:
		return a.bandwidth < b.bandwidth
	}
}
