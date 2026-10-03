package applemusic

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNormalize(t *testing.T) {
	tests := map[string]string{
		"Chaos A.D.":         "chaos a d",
		"  AC/DC ":           "ac dc",
		"Motörhead":          "motorhead",
		"Simon & Garfunkel":  "simon and garfunkel",
		"Beyoncé":            "beyonce",
		"(What's the Story)": "what s the story",
		"ベビーメタル":             "ベビーメタル",
		"--":                 "",
	}
	for in, want := range tests {
		if got := normalize(in); got != want {
			t.Errorf("normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestArtistMatches(t *testing.T) {
	tests := []struct {
		local, remote string
		want          bool
	}{
		{"Korn", "Korn", true},
		{"System of a Down", "System Of A Down", true},
		{"Beatles", "The Beatles", true},
		{"Motorhead", "Motörhead", true},
		{"Slash", "Slash feat. Myles Kennedy & The Conspirators", true},
		{"Korn", "Eric B. & Rakim", false},
		{"Sepultura", "Arkhanoid", false},
		{"", "Korn", false},
	}
	for _, tt := range tests {
		if got := artistMatches(tt.local, tt.remote); got != tt.want {
			t.Errorf("artistMatches(%q, %q) = %v, want %v", tt.local, tt.remote, got, tt.want)
		}
	}
}

func TestAlbumScore(t *testing.T) {
	tests := []struct {
		local, remote string
		want          int
	}{
		{"Follow the Leader", "Follow The Leader", 2},
		{"Chaos A.D.", "Chaos A.D. (Remastered)", 1},
		{"Toxicity", "Toxicity [Deluxe Edition]", 1},
		{"Protect the Land / Genocidal Humanoidz", "Protect the Land / Genocidal Humanoidz - Single", 1},
		{"(What's the Story) Morning Glory?", "(What's the Story) Morning Glory?", 2},
		{"Greatest Hits", "Greatest Hits Live", 0},
		{"Chaos A.D.", "The Chaos a.D. Essentials: Tribute to Sepultura - Single", 0},
	}
	for _, tt := range tests {
		if got := albumScore(tt.local, tt.remote); got != tt.want {
			t.Errorf("albumScore(%q, %q) = %d, want %d", tt.local, tt.remote, got, tt.want)
		}
	}
}

const searchFixture = `{"results":{"albums":{"data":[
 {"id":"1","attributes":{"artistName":"Sepultura","name":"Chaos A.D. (Expanded Edition)","editorialVideo":{}}},
 {"id":"2","attributes":{"artistName":"Arkhanoid","name":"Chaos A.D.","editorialVideo":{"motionDetailSquare":{"video":"https://example.com/wrong.m3u8"}}}},
 {"id":"3","attributes":{"artistName":"Sepultura","name":"Chaos A.D. (Remastered)","editorialVideo":{"motionDetailTall":{"video":"https://example.com/tall.m3u8"}}}},
 {"id":"4","attributes":{"artistName":"Sepultura","name":"Chaos A.D. [Deluxe]","editorialVideo":{"motionDetailSquare":{"video":"https://example.com/square.m3u8"}}}}
]}}}`

const freshToken = "eyJfresh.payload.signature"

// newTestClient starts a fake Apple Music that serves the token assets and
// answers searches with the given status and body. Requests carrying the
// expired token get 401.
func newTestClient(t *testing.T, status int, body string) *Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/us/browse", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<script src="/assets/index-test.js"></script>`))
	})
	mux.HandleFunc("/assets/index-test.js", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`const token="` + freshToken + `";`))
	})
	mux.HandleFunc("/v1/catalog/us/search", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("types") != "albums" || q.Get("extend") != "editorialVideo" || q.Get("term") != "Sepultura Chaos A.D." {
			t.Errorf("unexpected query %s", r.URL.RawQuery)
		}
		if r.Header.Get("Authorization") != "Bearer "+freshToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := NewClient()
	c.httpClient = srv.Client()
	c.webBase = srv.URL
	c.apiBase = srv.URL
	c.token = "eyJexpired.payload.signature"
	c.tokenTime = time.Now()
	return c
}

func TestSearchAnimatedCoverPicksMatchingSquareVideo(t *testing.T) {
	c := newTestClient(t, http.StatusOK, searchFixture)

	m, err := c.SearchAnimatedCover(context.Background(), "Sepultura", "Chaos A.D.")
	if err != nil {
		t.Fatal(err)
	}
	if m == nil || m.ID != "4" || m.VideoURL != "https://example.com/square.m3u8" {
		t.Fatalf("got %+v, want album 4 with square video", m)
	}
	if c.token != freshToken {
		t.Fatalf("token = %q, want the refreshed token", c.token)
	}
}

func TestSearchAnimatedCoverNoMatch(t *testing.T) {
	c := newTestClient(t, http.StatusOK, `{"results":{"albums":{"data":[
	 {"id":"2","attributes":{"artistName":"Arkhanoid","name":"Chaos A.D.","editorialVideo":{"motionDetailSquare":{"video":"https://example.com/wrong.m3u8"}}}}
	]}}}`)

	m, err := c.SearchAnimatedCover(context.Background(), "Sepultura", "Chaos A.D.")
	if err != nil || m != nil {
		t.Fatalf("got (%+v, %v), want (nil, nil)", m, err)
	}
}

func TestSearchAnimatedCoverRateLimited(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusForbidden} {
		c := newTestClient(t, status, "")

		_, err := c.SearchAnimatedCover(context.Background(), "Sepultura", "Chaos A.D.")
		if !errors.Is(err, ErrRateLimited) {
			t.Errorf("status %d: got %v, want ErrRateLimited", status, err)
		}
	}
}

func TestSearchAnimatedCoverServerError(t *testing.T) {
	c := newTestClient(t, http.StatusInternalServerError, "")

	m, err := c.SearchAnimatedCover(context.Background(), "Sepultura", "Chaos A.D.")
	if err == nil || errors.Is(err, ErrRateLimited) || m != nil {
		t.Fatalf("got (%+v, %v), want a generic error", m, err)
	}
}
