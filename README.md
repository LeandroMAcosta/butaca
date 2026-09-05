# butaca

One catalog for movies, series and subtitles. Replaces Radarr, Sonarr and
Bazarr with a single service; keeps Prowlarr (indexer aggregation) and
qBittorrent (downloading), because those two are worth delegating to.

**Status: M1.** Movies work end to end — add, search, decide, grab, import,
subtitles. Series (M4), the MCP server (M3) and the TUI (M5) are not built yet.

## Why

The stack it replaces has a decision engine that cannot express what you
actually want. Its quality profile carries a single fixed audio language, so a
profile set to English rejects every film whose original language is not
English. Amélie, Exit 8 and Christiane F. all had to be searched and grabbed by
hand for that reason.

Here the language rule is relative to the film:

```yaml
rules:
  language_mode: original   # match the film's own original language
```

`original` compares each release against TMDB's `original_language`, so Amélie
wants French, Exit 8 wants Japanese and Taxi Driver wants English, with no
per-film intervention. `prefer` and `any` are also available.

## How it fits together

```
      CLI / TUI ─┐
                 ├─► butaca ─┬─► Prowlarr    (search, all trackers at once)
   Claude (MCP) ─┘           ├─► qBittorrent (download)
                             └─► butaca-parse (guessit + subliminal)

            SQLite · /media/movies · /media/tv · /media/downloads
```

`butaca-parse` is a small Python sidecar. Go owns state, concurrency and the
interfaces; Python owns the two libraries not worth porting — `guessit` for
release-name parsing and `subliminal` for subtitles.

Imports are **hardlinks**, never copies: the file keeps seeding under its
download name while appearing in the library under a clean one, at no extra
disk cost. The corollary is that deleting one name frees nothing, which is why
`butaca rm` tears down the catalog row, the library folder and the torrent
together.

## Quick start

```sh
cp .env.example .env      # add your Prowlarr API key
docker compose up -d
```

Or run it directly:

```sh
go build -o butaca ./cmd/butaca

# the sidecar
cd sidecar && uv run --with fastapi --with 'uvicorn[standard]' \
  --with guessit --with subliminal --with babelfish \
  uvicorn app:app --port 8000

export PROWLARR_API_KEY=...
./butaca config init
./butaca status
```

## Usage

```sh
butaca add "Taxi Driver" --year 1976        # add and search
butaca add "Amélie" --lang fr --alt-title "Le Fabuleux Destin d'Amélie Poulain"
butaca search Amélie --explain              # every release, scored, with reasons
butaca search Amélie --grab                 # send the winner to qBittorrent
butaca import --watch                       # import finished downloads
butaca list
butaca rm "Taxi Driver"                     # catalog + disk + torrent
butaca status                               # dependency health and queue
butaca config set paths.movies /media/movies
```

`--alt-title` matters for foreign-language films: releases are usually named
after the original title. With a TMDB key it is filled in automatically.

`--explain` prints every candidate with its score and, when rejected, why:

```
ok       232  Christiane F. Wir Kinder vom Bahnhof Zoo (1981) 1080p BRRip 5.1 x264
             1080p | 2.41 GB | 35 seeders | Knaben
REJECT   149  Le Fabuleux Destin d'Amélie Poulain (2001) 720p BRRip x264 -YTS
             720p | 1.00 GB | 71 seeders | YTS
             - resolution 720p not in 1080p
```

## Configuration

`~/.config/butaca/config.yaml`, with `BUTACA_*` environment overrides.

```yaml
paths:
  movies: /media/movies
  tv: /media/tv
  downloads: /media/downloads   # must share a filesystem with the two above
rules:
  min_seeders: 5
  resolutions: ["1080p"]        # ordered, best first
  sources: ["Blu-ray", "Web", "HDTV", "DVD"]
  min_size: 500MB
  max_size: 20GB
  language_mode: original
subtitles:
  languages: ["es"]
  auto: true
```

`butaca status` reports whether `downloads` and `movies` actually share a
filesystem. If they do not, imports fail loudly rather than silently copying and
doubling disk usage.

## Tests

```sh
go test ./...                                  # unit tests
BUTACA_IT=1 go test ./internal/app/            # against live qBittorrent
```

The decision-engine tests use real release names captured from Prowlarr,
including the three films the previous stack rejected.
