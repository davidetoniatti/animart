package applemusic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	jsAssetRegex   = regexp.MustCompile(`(/assets/[^"]+\.js)`)
	jwtRegex       = regexp.MustCompile(`["'](eyJ[A-Za-z0-9-_=]+\.[A-Za-z0-9-_=]+\.[A-Za-z0-9-_=]+)["']`)
	albumLinkRegex = regexp.MustCompile(`href="(https://music\.apple\.com/([a-z]{2})/album/([^/"?]+)/(\d+))"`)
	nonAlphaRegex  = regexp.MustCompile(`[^a-z0-9]+`)
)

type Client struct {
	httpClient *http.Client
	mu         sync.RWMutex
	token      string
	tokenTime  time.Time
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (c *Client) getBearerToken(ctx context.Context) (string, error) {
	c.mu.RLock()
	if c.token != "" && time.Since(c.tokenTime) < 12*time.Hour {
		t := c.token
		c.mu.RUnlock()
		return t, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double check
	if c.token != "" && time.Since(c.tokenTime) < 12*time.Hour {
		return c.token, nil
	}

	req, err := http.NewRequestWithContext(ctx, "GET", "https://music.apple.com/us/browse", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch Apple Music browse page: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read browse body: %w", err)
	}

	matches := jsAssetRegex.FindAllStringSubmatch(string(body), -1)
	for _, m := range matches {
		path := m[1]
		if !strings.Contains(path, "index") && !strings.Contains(path, "web-client") && !strings.Contains(path, "apple-music") {
			continue
		}

		jsURL := "https://music.apple.com" + path
		jsReq, err := http.NewRequestWithContext(ctx, "GET", jsURL, nil)
		if err != nil {
			continue
		}
		jsReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

		jsResp, err := c.httpClient.Do(jsReq)
		if err != nil {
			continue
		}
		jsBody, _ := io.ReadAll(jsResp.Body)
		jsResp.Body.Close()

		tokenMatch := jwtRegex.FindStringSubmatch(string(jsBody))
		if len(tokenMatch) > 1 {
			c.token = tokenMatch[1]
			c.tokenTime = time.Now()
			return c.token, nil
		}
	}

	return "", fmt.Errorf("could not extract Apple Music developer token from javascript assets")
}

func normalize(s string) string {
	s = strings.ToLower(s)
	s = nonAlphaRegex.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func (c *Client) SearchAnimatedCover(ctx context.Context, artist, album string) (videoURL string, found bool, err error) {
	token, err := c.getBearerToken(ctx)
	if err != nil {
		return "", false, err
	}

	storefront, albumID, err := c.searchAlbum(ctx, artist, album)
	if err != nil {
		// Try fallback to iTunes Search API
		storefront, albumID, err = c.searchItunes(ctx, artist, album)
		if err != nil {
			return "", false, nil // Not found on Apple Music
		}
	}

	apiURL := fmt.Sprintf("https://amp-api.music.apple.com/v1/catalog/%s/albums/%s?extend=editorialVideo&platform=web", storefront, albumID)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Origin", "https://music.apple.com")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return "", false, fmt.Errorf("apple music rate limited (429)")
	}

	if resp.StatusCode != http.StatusOK {
		return "", false, nil
	}

	var data struct {
		Data []struct {
			Attributes struct {
				EditorialVideo struct {
					MotionDetailSquare struct {
						Video string `json:"video"`
					} `json:"motionDetailSquare"`
					MotionDetailTall struct {
						Video string `json:"video"`
					} `json:"motionDetailTall"`
				} `json:"editorialVideo"`
			} `json:"attributes"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", false, err
	}

	if len(data.Data) > 0 {
		ev := data.Data[0].Attributes.EditorialVideo
		if ev.MotionDetailSquare.Video != "" {
			return ev.MotionDetailSquare.Video, true, nil
		}
		if ev.MotionDetailTall.Video != "" {
			return ev.MotionDetailTall.Video, true, nil
		}
	}

	return "", false, nil
}

func (c *Client) searchAlbum(ctx context.Context, artist, album string) (storefront, albumID string, err error) {
	query := url.QueryEscape(artist + " " + album)
	searchURL := "https://music.apple.com/us/search?term=" + query

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("search status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	matches := albumLinkRegex.FindAllStringSubmatch(string(body), -1)
	if len(matches) == 0 {
		return "", "", fmt.Errorf("no album link matches found")
	}

	normAlbum := normalize(album)

	// Try finding best matching slug
	for _, m := range matches {
		slug, _ := url.QueryUnescape(m[3])
		slugNorm := normalize(strings.ReplaceAll(slug, "-", " "))
		if strings.Contains(slugNorm, normAlbum) || strings.Contains(normAlbum, slugNorm) {
			return m[2], m[4], nil
		}
	}

	// Fallback to first result
	return matches[0][2], matches[0][4], nil
}

func (c *Client) searchItunes(ctx context.Context, artist, album string) (storefront, albumID string, err error) {
	query := url.QueryEscape(artist + " " + album)
	itunesURL := "https://itunes.apple.com/search?term=" + query + "&entity=album&limit=3"

	req, err := http.NewRequestWithContext(ctx, "GET", itunesURL, nil)
	if err != nil {
		return "", "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	var result struct {
		ResultCount int `json:"resultCount"`
		Results     []struct {
			CollectionID   int64  `json:"collectionId"`
			CollectionName string `json:"collectionName"`
			ArtistName     string `json:"artistName"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", err
	}

	if result.ResultCount > 0 {
		return "us", fmt.Sprintf("%d", result.Results[0].CollectionID), nil
	}

	return "", "", fmt.Errorf("no itunes results")
}
