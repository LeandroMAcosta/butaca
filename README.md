# butaca

One catalog for movies, series and subtitles. Replaces Radarr, Sonarr and
Bazarr with a single service; keeps Prowlarr (indexer aggregation) and
qBittorrent (downloading), because those two are worth delegating to.

Movies and series, a decision engine you can actually express your taste in,
subtitles, a CLI, a TUI, an MCP server, and a background service.

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

./butaca setup          # asks where media lives, which language, which subtitles
./butaca status
```

The wizard runs automatically the first time butaca is used interactively.
Non-interactive runs (Docker, cron, MCP) skip it and use defaults and
environment variables instead.

## Migrating from Radarr

```sh
butaca migrate --dry-run    # preview
butaca migrate              # apply
butaca orphans              # downloads no catalog entry points at
```

Nothing is moved, copied or deleted — only the catalog is written, so it is
safe to run while the old stack is still installed. Radarr stores the original
language as an integer in its own enum, which `migrate` maps back to ISO codes
so `language_mode: original` keeps working for the imported films.

## Running it as a service

```sh
butaca serve            # import every minute, search every 6h, subtitles every 12h
butaca serve --search-interval 0    # disable a job
```

Each job has its own ticker, so a slow search never delays an import.

## Driving it from Claude

```sh
claude mcp add butaca -- /path/to/butaca mcp
```

Eight tools: `list`, `add`, `search`, `grab`, `remove`, `status`, `subtitles`,
`import`. `remove` is one call that clears the catalog row, the library folder
and the torrent together — with hardlinks, doing only one of the three frees no
disk space.

## Usage

```sh
butaca tui                                  # browse the catalog and queue
butaca add "Taxi Driver" --year 1976        # add and search
butaca add "Severance" --series             # series need a TMDB key
butaca add "Amélie" --lang fr --alt-title "Le Fabuleux Destin d'Amélie Poulain"
butaca search Amélie --explain              # every release, scored, with reasons
butaca search Amélie --grab                 # send the winner to qBittorrent
butaca import --watch                       # import finished downloads
butaca list
butaca rm "Taxi Driver"                     # catalog + disk + torrent
butaca status                               # dependency health and queue
butaca config set paths.movies /media/movies
butaca setup                                # re-run the wizard
butaca migrate                              # import a Radarr catalog
butaca orphans                              # unaccounted-for downloads
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

## Series

```sh
butaca add "Severance" --series
```

Seasons and episodes come from TMDB, so series need a key. Each aired,
monitored episode is searched individually and imported as:

```
Severance (2022)/Season 02/Severance (2022) - S02E10 - Cold Harbor.mkv
```

Season packs are rejected for single-episode searches: they need different
import handling, and mixing the two silently produces wrong filenames.

## Tests

```sh
go test ./...                                  # unit tests
BUTACA_IT=1 go test ./internal/app/            # against live qBittorrent
```

The decision-engine tests use real release names captured from Prowlarr,
including the three films the previous stack rejected.
