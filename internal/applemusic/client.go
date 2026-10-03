package applemusic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const (
	defaultWebBase    = "https://music.apple.com"
	defaultAPIBase    = "https://amp-api.music.apple.com"
	defaultStorefront = "us"
	searchLimit       = 10
)

var (
	jsAssetRegex   = regexp.MustCompile(`(/assets/[^"]+\.js)`)
	jwtRegex       = regexp.MustCompile(`["'](eyJ[A-Za-z0-9-_=]+\.[A-Za-z0-9-_=]+\.[A-Za-z0-9-_=]+)["']`)
	qualifierRegex = regexp.MustCompile(`\s*[\(\[][^\)\]]*[\)\]]`)
	releaseSuffix  = regexp.MustCompile(`(?i)\s+-\s+(single|ep)$`)
)

// ErrRateLimited reports that Apple Music rejected a request with 429 or 403.
var ErrRateLimited = errors.New("apple music rate limited")

var errUnauthorized = errors.New("apple music rejected the token")

type Client struct {
	httpClient *http.Client
	webBase    string
	apiBase    string
	storefront string
	mu         sync.RWMutex
	token      string
	tokenTime  time.Time
}

// Match is the Apple Music album chosen for a local album.
type Match struct {
	ID       string
	Artist   string
	Album    string
	VideoURL string
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		webBase:    defaultWebBase,
		apiBase:    defaultAPIBase,
		storefront: defaultStorefront,
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

	req, err := http.NewRequestWithContext(ctx, "GET", c.webBase+"/us/browse", nil)
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

		jsURL := c.webBase + path
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

func (c *Client) resetToken() {
	c.mu.Lock()
	c.token = ""
	c.mu.Unlock()
}

// foldLatinDiacritics removes accents from Latin letters (ö -> o) and keeps
// combining marks of other scripts, which are part of the letter (ベ != ヘ).
func foldLatinDiacritics(s string) string {
	var b strings.Builder
	var base rune
	for _, r := range norm.NFD.String(s) {
		if unicode.Is(unicode.Mn, r) {
			if unicode.Is(unicode.Latin, base) {
				continue
			}
		} else {
			base = r
		}
		b.WriteRune(r)
	}
	return norm.NFC.String(b.String())
}

// normalize lowercases s, folds diacritics, maps "&" to "and" and collapses
// every run of non letter/digit characters into a single space.
func normalize(s string) string {
	s = foldLatinDiacritics(strings.ToLower(strings.ReplaceAll(s, "&", " and ")))

	var b strings.Builder
	pendingSpace := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if pendingSpace && b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteRune(r)
			pendingSpace = false
		} else {
			pendingSpace = true
		}
	}
	return b.String()
}

// stripQualifiers removes edition markers such as "(Remastered)",
// "[Deluxe Edition]" and the " - Single" / " - EP" suffixes.
func stripQualifiers(s string) string {
	s = releaseSuffix.ReplaceAllString(strings.TrimSpace(s), "")
	return strings.TrimSpace(qualifierRegex.ReplaceAllString(s, ""))
}

func normalizeArtist(s string) string {
	return strings.TrimPrefix(normalize(s), "the ")
}

func containsWords(s, sub string) bool {
	return strings.Contains(" "+s+" ", " "+sub+" ")
}

// artistMatches accepts equal names or one name contained word-wise in the
// other ("Slash" matches "Slash feat. Myles Kennedy").
func artistMatches(local, remote string) bool {
	a, b := normalizeArtist(local), normalizeArtist(remote)
	if a == "" || b == "" {
		return false
	}
	return a == b || containsWords(a, b) || containsWords(b, a)
}

// albumScore returns 2 for equal names, 1 for names equal after removing
// edition qualifiers, 0 otherwise.
func albumScore(local, remote string) int {
	if a := normalize(local); a != "" && a == normalize(remote) {
		return 2
	}
	if a := normalize(stripQualifiers(local)); a != "" && a == normalize(stripQualifiers(remote)) {
		return 1
	}
	return 0
}

type searchResponse struct {
	Results struct {
		Albums struct {
			Data []struct {
				ID         string `json:"id"`
				Attributes struct {
					ArtistName     string `json:"artistName"`
					Name           string `json:"name"`
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
		} `json:"albums"`
	} `json:"results"`
}

// SearchAnimatedCover searches the Apple Music catalog for the album and
// returns the best matching edition that has an animated cover. It returns
// (nil, nil) when no matching album with an animated cover exists.
func (c *Client) SearchAnimatedCover(ctx context.Context, artist, album string) (*Match, error) {
	term := artist + " " + album
	data, err := c.search(ctx, term)
	if errors.Is(err, errUnauthorized) {
		c.resetToken()
		data, err = c.search(ctx, term)
	}
	if err != nil {
		return nil, err
	}
	return bestMatch(data, artist, album), nil
}

func (c *Client) search(ctx context.Context, term string) (*searchResponse, error) {
	token, err := c.getBearerToken(ctx)
	if err != nil {
		return nil, err
	}

	q := url.Values{}
	q.Set("term", term)
	q.Set("types", "albums")
	q.Set("limit", fmt.Sprint(searchLimit))
	q.Set("extend", "editorialVideo")
	apiURL := fmt.Sprintf("%s/v1/catalog/%s/search?%s", c.apiBase, c.storefront, q.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Origin", "https://music.apple.com")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return nil, errUnauthorized
	case http.StatusTooManyRequests, http.StatusForbidden:
		return nil, ErrRateLimited
	default:
		return nil, fmt.Errorf("apple music search status: %d", resp.StatusCode)
	}

	var data searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return &data, nil
}

// bestMatch ranks matching albums by album score first, square video over
// tall video second and search relevance (result order) last.
func bestMatch(data *searchResponse, artist, album string) *Match {
	var best *Match
	bestRank := -1
	for _, d := range data.Results.Albums.Data {
		attrs := d.Attributes
		if !artistMatches(artist, attrs.ArtistName) {
			continue
		}
		score := albumScore(album, attrs.Name)
		if score == 0 {
			continue
		}

		video, rank := attrs.EditorialVideo.MotionDetailSquare.Video, score*2+1
		if video == "" {
			video, rank = attrs.EditorialVideo.MotionDetailTall.Video, score*2
		}
		if video == "" || rank <= bestRank {
			continue
		}

		best = &Match{ID: d.ID, Artist: attrs.ArtistName, Album: attrs.Name, VideoURL: video}
		bestRank = rank
	}
	return best
}
