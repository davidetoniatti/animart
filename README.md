# animart

[![CI](https://github.com/davidetoniatti/animart/actions/workflows/ci.yml/badge.svg)](https://github.com/davidetoniatti/animart/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/davidetoniatti/animart.svg)](https://pkg.go.dev/github.com/davidetoniatti/animart)
[![Go Report Card](https://goreportcard.com/badge/github.com/davidetoniatti/animart)](https://goreportcard.com/report/github.com/davidetoniatti/animart)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Animated album covers from Apple Music for a local music library, written in Go. animart scans
the library, finds each album on Apple Music and saves its animated artwork as `cover.webp` in
the album folder, ready for Navidrome and Subsonic clients such as Symfonium and Feishin.

## Features

- Reads artist and album from the tags of FLAC, MP3, M4A, OGG, Opus, AAC, WAV and WMA files
- Matches albums by artist and title, ignoring case, punctuation, accents and edition markers
  such as `(Remastered)` or `[Deluxe Edition]`
- Square animated WebP: smallest HLS variant that fits the output size, center crop, scaling,
  reduced frame rate (default 800 px, 20 fps)
- SQLite cache: albums without animated artwork are skipped for 30 days, failed lookups are retried
- Backs off when Apple Music rate-limits requests
- One-shot or daemon mode with periodic rescans
- Folder exclusions, any Apple Music storefront, forced re-encoding of existing covers
- Single static binary; needs only `ffmpeg`

## Requirements

`ffmpeg` with the `libwebp` encoder in `PATH`:

```sh
ffmpeg -hide_banner -encoders | grep libwebp
```

The Docker image includes it.

## Installation

### Go

```sh
go install github.com/davidetoniatti/animart@latest
```

Requires Go 1.27 or newer. The binary is installed in `$(go env GOPATH)/bin`.

### Binaries

Pre-built binaries for Linux, macOS and Windows (amd64, arm64) are attached to each
[release](https://github.com/davidetoniatti/animart/releases).

### Docker

```sh
docker pull ghcr.io/davidetoniatti/animart:latest
```

[`compose.example.yml`](compose.example.yml) runs it as a daemon: set the music library path and
start it with `docker compose up -d`. The cache is stored in `/data/cache.db`.

### From source

```sh
git clone https://github.com/davidetoniatti/animart.git
cd animart
make build      # ./animart
make install    # $(go env GOPATH)/bin/animart
```

## Usage

```sh
animart -music /path/to/music -once                       # one scan, then exit
animart -music /path/to/music -exclude explo,discover     # daemon, rescans every 24h
animart -music /path/to/music -storefront it -interval 12h
animart -music /path/to/music -once -force                # re-encode existing covers
```

Folders that already contain `cover.webp` are skipped, unless `-force` is set.

### Flags

| Flag | Description | Default |
| --- | --- | --- |
| `-music` | Music library root (required) | |
| `-exclude` | Comma-separated folder names to skip, at any depth, case-insensitive | |
| `-cover-name` | File name of the cover in each album folder | `cover.webp` |
| `-cache` | SQLite cache path | `~/.cache/animart/cache.db` |
| `-storefront` | Apple Music storefront (two-letter country code) | `us` |
| `-delay-ms` | Pause after each Apple Music lookup | `2000` |
| `-negative-ttl` | How long an album without animated artwork is skipped (`0` = forever) | `720h` |
| `-scale` | Width and height of the cover in pixels | `800` |
| `-fps` | Frame rate of the cover | `20` |
| `-quality` | WebP quality, 0-100 | `50` |
| `-interval` | Time between scans (`0` = run once) | `24h` |
| `-once` | Run one scan and exit | `false` |
| `-force` | Replace existing covers; requires `-once` | `false` |
| `-verbose` | Log albums skipped through the cache | `false` |
| `-version` | Print the version and exit | |

The default cache path follows the platform cache directory (`$XDG_CACHE_HOME` on Linux,
`~/Library/Caches` on macOS, `%LocalAppData%` on Windows).

## How it works

1. Walks the library and takes every folder with an audio file and no `cover.webp`.
2. Reads album artist (or artist) and album from the tags.
3. Gets the anonymous token of the Apple Music web player from its public JavaScript.
4. Searches the Apple Music catalog and keeps the edition whose artist and title match and that
   has an animated cover, preferring the square video.
5. Converts the HLS stream to animated WebP with `ffmpeg` and moves it into place atomically.
6. Records the result in the cache. On `429` or `403` it waits 1, 2, 4, 8 and 16 minutes before
   giving up until the next scan.

## Development

```sh
make check    # gofmt, go vet, go test -race
make build
```

Tests run against in-process stubs of Apple Music; nothing in the test suite touches the
network. CI runs on Linux, macOS and Windows. Tagging `vX.Y.Z` publishes release binaries with
GoReleaser and the Docker image to GitHub Container Registry.

## Disclaimer

animart uses undocumented Apple Music endpoints, which may change or stop working at any time.
This project is not affiliated with Apple. The approach follows
[m8tec/apple-music-animated-artworks](https://github.com/m8tec/apple-music-animated-artworks).

## License

[MIT](LICENSE)
