# AnimArt

Downloads Apple Music animated album artwork for a local music library and saves it as an animated `cover.webp` in each album folder, for use with Navidrome and Subsonic clients (Symfonium, Feishin, etc.).

AnimArt is a single Go binary. It is a reimplementation of the two-container .NET stack [m8tec/animated-artwork-downloader](https://github.com/m8tec/animated-artwork-downloader) + [m8tec/apple-music-animated-artworks](https://github.com/m8tec/apple-music-animated-artworks), without the HTTP API and web UI.

> [!NOTE]
> AnimArt is not affiliated with Apple. It uses the anonymous token of the Apple Music web player and undocumented endpoints, which may change or stop working at any time.

## How it works

1. Walks the library and selects every folder that contains audio files (`.flac`, `.mp3`, `.m4a`, `.ogg`, `.opus`, `.aac`, `.wav`, `.wma`) and no `cover.webp`.
2. Reads album artist (or artist) and album from the tags of the first readable audio file.
3. Fetches the anonymous Apple Music web player token from the public JavaScript assets of `music.apple.com`.
4. Searches the Apple Music catalog (`amp-api.music.apple.com`, `types=albums`, `extend=editorialVideo`) and keeps only results whose artist and album match the tags. Matching ignores case, punctuation, Latin diacritics, a leading "The" and edition markers such as `(Remastered)`, `[Deluxe Edition]` or ` - EP`. Among the matching editions, an exact title match wins over a qualified one and a square video over a tall one.
5. Picks the smallest HLS variant whose sides are at least `-scale` pixels, preferring H.264, converts it to animated WebP with `ffmpeg` (scaled, frame rate reduced, looped) and moves it atomically into place.
6. Stores positive and negative results in a local SQLite cache, so later scans skip albums that were already checked. Negative results expire after `-negative-ttl`; failed lookups are not stored and run again on the next scan. When Apple Music answers 429 or 403, the lookup waits and retries (1, 2, 4, 8, 16 minutes) and the cycle ends if the limit persists.

## Requirements

- `ffmpeg` with the `libwebp` encoder in `PATH` (check with `ffmpeg -hide_banner -encoders | grep libwebp`). Not needed with the Docker image.
- Go 1.27+ only to build from source.

## Installation

### From source

```bash
go install github.com/davidetoniatti/animart@latest
```

or

```bash
git clone https://github.com/davidetoniatti/animart.git
cd animart
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o animart .
```

The result is a static binary (~11 MB). SQLite is provided by `modernc.org/sqlite` (pure Go, no CGO).

### Docker

```bash
cp compose.example.yml compose.yml   # set the music library path
docker compose up -d
docker compose logs -f
```

The image contains `ffmpeg`. The cache is written to `/data/cache.db`.

## Usage

Single run:

```bash
animart -music /path/to/music -exclude "explo,discover" -once
```

Daemon with a scan every 24 hours:

```bash
animart -music /path/to/music -interval 24h
```

### Flags

| Flag | Description | Default |
| :--- | :--- | :--- |
| `-music` | Music library root (required) | — |
| `-exclude` | Comma-separated folder names to skip, matched at any depth, case-insensitive | none |
| `-cover-name` | Output file name in each album folder | `cover.webp` |
| `-cache` | SQLite cache path | `$XDG_CACHE_HOME/animart/cache.db` (`~/.cache/animart/cache.db`) |
| `-delay-ms` | Delay after each Apple Music lookup | `2000` |
| `-negative-ttl` | How long an album without animated artwork is skipped before a new lookup (`0` = forever) | `720h` |
| `-scale` | Maximum width/height in pixels | `800` |
| `-fps` | Output frame rate | `20` |
| `-quality` | WebP quality (0–100) | `50` |
| `-interval` | Rescan interval (`24h`, `12h`, `0` to run once) | `24h` |
| `-once` | Run one scan and exit | `false` |
| `-verbose` | Log albums skipped through the negative cache | `false` |

Albums that already have a `cover.webp` are never processed again. To regenerate a cover, delete the file.

## Comparison with the .NET stack

Measured on the same host (4 cores, x86_64), album *Korn – Follow the Leader*, default settings of each tool.

| | .NET stack (2 containers) | AnimArt |
| :--- | :--- | :--- |
| Distribution size | 962 MB (Docker images: 647 MB + 315 MB) | 11 MB static binary + system `ffmpeg`; Docker image 154 MB (Alpine + ffmpeg) |
| Resident memory of the service | 54 MiB + 32 MiB | 31 MiB |
| Output settings | source ≥ 1000 px (1080×1080), 30 fps, `compression_level 6` | 800 px, 20 fps, `compression_level 4` |
| `cover.webp` size | 43.7 MB | 5.0 MB |
| Library scan (322 folders, 5,481 files, warm cache) | — | 35–50 ms |
| Folder exclusion | `tmpfs` mount over the folder | `-exclude` flag |
| HTTP API / web UI | yes | no |

Most of the difference in conversion time and output size comes from the output settings (resolution, frame rate, compression level), not from the language: both tools delegate encoding to `ffmpeg`.

## Credits

The token extraction and album lookup approach follow [m8tec/apple-music-animated-artworks](https://github.com/m8tec/apple-music-animated-artworks).

## License

[MIT](LICENSE)
