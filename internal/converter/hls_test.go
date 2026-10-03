package converter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

const masterFixture = `#EXTM3U
#EXT-X-I-FRAME-STREAM-INF:BANDWIDTH=365829,CODECS="avc1.64001f",RESOLUTION=960x960,URI="iframes_960.m3u8"
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=2176816,BANDWIDTH=2798634,CODECS="avc1.64001f",RESOLUTION=768x768
avc_768.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=1865392,BANDWIDTH=2311718,CODECS="hvc1.2.20000000.L123.B0",RESOLUTION=768x768
hevc_768.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=3922648,BANDWIDTH=5115608,CODECS="avc1.64001f",RESOLUTION=960x960
avc_960_high.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=2897972,BANDWIDTH=3724548,CODECS="avc1.64001f",RESOLUTION=960x960
https://cdn.example.com/avc_960_low.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=2652607,BANDWIDTH=3196454,CODECS="hvc1.2.20000000.L123.B0",RESOLUTION=960x960
hevc_960.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=3939276,BANDWIDTH=5107640,CODECS="avc1.640020",RESOLUTION=1080x1080
avc_1080.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=14577916,BANDWIDTH=15157864,CODECS="hvc1.2.20000000.H150.B0",RESOLUTION=2160x2160
hevc_2160.m3u8
`

func TestParseVariants(t *testing.T) {
	base, _ := url.Parse("https://example.com/hls/master.m3u8")
	vs := parseVariants(base, masterFixture)

	if len(vs) != 7 {
		t.Fatalf("got %d variants, want 7", len(vs))
	}
	want := variant{url: "https://example.com/hls/avc_768.m3u8", width: 768, height: 768, bandwidth: 2798634, avc: true}
	if vs[0] != want {
		t.Errorf("got %+v, want %+v", vs[0], want)
	}
	if vs[3].url != "https://cdn.example.com/avc_960_low.m3u8" {
		t.Errorf("absolute URI resolved to %s", vs[3].url)
	}
	if vs[1].avc {
		t.Errorf("HEVC variant parsed as AVC")
	}
}

func TestPickVariant(t *testing.T) {
	base, _ := url.Parse("https://example.com/hls/master.m3u8")
	vs := parseVariants(base, masterFixture)

	tests := []struct {
		scale int
		want  string
	}{
		{800, "https://cdn.example.com/avc_960_low.m3u8"},
		{768, "https://example.com/hls/avc_768.m3u8"},
		{1000, "https://example.com/hls/avc_1080.m3u8"},
		{1200, "https://example.com/hls/hevc_2160.m3u8"},
		{4000, "https://example.com/hls/hevc_2160.m3u8"},
	}
	for _, tt := range tests {
		if got := pickVariant(vs, tt.scale).url; got != tt.want {
			t.Errorf("scale %d: got %s, want %s", tt.scale, got, tt.want)
		}
	}
}

func TestResolveVariant(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/master.m3u8":
			_, _ = w.Write([]byte(masterFixture))
		case "/media.m3u8":
			_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:6.0,\nsegment0.mp4\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := &Converter{httpClient: srv.Client()}
	ctx := context.Background()

	if got, err := c.resolveVariant(ctx, srv.URL+"/master.m3u8", 800); err != nil || got != "https://cdn.example.com/avc_960_low.m3u8" {
		t.Errorf("master: got (%s, %v)", got, err)
	}
	if got, err := c.resolveVariant(ctx, srv.URL+"/media.m3u8", 800); err != nil || got != srv.URL+"/media.m3u8" {
		t.Errorf("media playlist: got (%s, %v)", got, err)
	}
	if got, err := c.resolveVariant(ctx, srv.URL+"/video.mp4", 800); err != nil || got != srv.URL+"/video.mp4" {
		t.Errorf("non-HLS URL: got (%s, %v)", got, err)
	}
	if _, err := c.resolveVariant(ctx, srv.URL+"/missing.m3u8", 800); err == nil {
		t.Error("missing playlist: want error")
	}
}
