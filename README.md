# butaca

One catalog for movies, series and subtitles. Replaces Radarr, Sonarr and
Bazarr with a single service; keeps Prowlarr (indexer aggregation) and
qBittorrent (downloading), because those two are worth delegating to.

Movies and series, a decision engine you can actually express your taste in,
subtitles, profiles, a watchlist, Letterboxd import, recommendations, a CLI, a
TUI, an MCP server, and a background service.

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

butaca is the brain, not the muscle. It needs three things running before it is
useful, in this order:

### 1. qBittorrent — the main dependency

Every download goes through it, and it must see the media tree at the **same
absolute path** butaca does (imports are hardlinks). Install it natively:

- **macOS**: the Homebrew cask is disabled (unsigned app, since 2026-09-01) and
  running it inside Docker Desktop breaks networking for every container (see
  below). Download the dmg from the [qBittorrent releases](https://github.com/qbittorrent/qBittorrent/releases),
  copy `qbittorrent.app` to `/Applications`, then
  `xattr -dr com.apple.quarantine /Applications/qBittorrent.app`.
- **Linux**: `apt install qbittorrent-nox` (or your distro's package) and run it
  as a service.

Then, in its settings (or `qBittorrent.ini` before first launch):

```ini
[Preferences]
WebUI\Enabled=true
WebUI\Port=8080
WebUI\LocalHostAuth=false     ; butaca talks to it from localhost, no password needed
[BitTorrent]
Session\DefaultSavePath=/path/to/media/downloads
```

### 2. Prowlarr — one search API for every tracker

Docker is fine for this one:

```sh
docker run -d --name prowlarr -p 9696:9696 -v prowlarr-config:/config lscr.io/linuxserver/prowlarr
```

Open `http://localhost:9696`, add your indexers, copy the API key from
Settings → General. Point `prowlarr.url` at a routable address, not
`localhost`, if qBittorrent could ever be in a container.

### 3. butaca-parse — the Python sidecar

Owns `guessit` (release-name parsing), `subliminal` (subtitles), `ffsubsync`
and `alass` (subtitle sync) and `ffprobe`. It needs `ffmpeg` on the PATH. `alass`
is optional; without it the sidecar only uses ffsubsync:

```sh
brew install ffmpeg alass          # Linux: apt install ffmpeg; alass from its GitHub releases
cd sidecar && uv run --python 3.12 --with-requirements requirements.txt \
  uvicorn app:app --port 8000
```

### 4. butaca

```sh
go build -o butaca ./cmd/butaca
./butaca setup          # asks where media lives, which language, which subtitles
./butaca status         # every dependency must say ok, and hardlinks ok
```

The wizard runs automatically the first time butaca is used interactively.
Non-interactive runs (Docker, cron, MCP) skip it and use defaults and
environment variables instead.

A TMDB API key (free, themoviedb.org → Settings → API) is optional but
recommended: without it `add` needs `--lang`, and series and recommendations do
not work at all.

### All in Docker (Linux hosts)

```sh
cp .env.example .env      # add your Prowlarr API key
docker compose up -d
```

`docker-compose.yml` runs butaca, the sidecar and Prowlarr together, bind-mounting
`MEDIA_ROOT` at its own host path so hardlinks and qBittorrent's reported paths keep
working. qBittorrent is still native even here. Do not use this on macOS: see the
next section.

### Running qBittorrent or Prowlarr in Docker while butaca is native

Four things bite in that mix, all of them silent:

- **Point `prowlarr.url` at a routable address, not `localhost`.** Prowlarr builds its
  download URLs from the host you queried it on, so `localhost` produces links that mean
  nothing inside the qBittorrent container and every grab fails.
- **Mount the media tree at the same absolute path in the container as on the host.**
  qBittorrent reports the path it saved to and butaca then stats it locally, so a
  container-only `/media` fails every import with `no such file or directory`.
- **qBittorrent's localhost auth bypass will not apply.** Reached through a published
  port, the container sees the Docker gateway rather than localhost, so either set
  credentials or whitelist that subnet in `WebUI\AuthSubnetWhitelist`.
- **On macOS, do not put qBittorrent inside Docker Desktop at all.** Its VM engine
  (`com.docker.sailor`) proxies every guest flow with a host thread, and macOS caps a
  process at 4096 threads (`kern.num_taskthreads`). qBittorrent's DHT opens thousands
  of UDP flows, the cap is hit within about half an hour, and from then on every
  container's outbound connection is refused: Prowlarr reports all indexers down,
  Pi-hole stops resolving, `butaca status` still says `prowlarr ok`. Restarting Docker
  only resets the counter. Run qBittorrent natively; the config it needs is
  `WebUI\Enabled=true`, `WebUI\LocalHostAuth=false` and the same `save_path`.

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

On macOS, run both `serve` and the sidecar under launchd rather than from a
terminal. Started from a shell, they die with it, and from then on finished
downloads sit in qBittorrent without being imported:

```sh
deploy/launchd/install.sh             # build, then install com.butaca.serve and com.butaca.parse
deploy/launchd/install.sh uninstall   # stop and remove both
```

Both start at login and restart if they exit. Logs go to `~/Library/Logs/butaca/`.
The sidecar listens on `127.0.0.1:8001`, so `parse.url` must be `http://localhost:8001`.
Stop any sidecar you started by hand first, or the agent cannot bind the port.

## Driving it from Claude

```sh
claude mcp add butaca -- /path/to/butaca mcp
```

Seventeen tools: `list`, `find`, `add`, `search`, `grab`, `remove`, `move`, `import`, `status`,
`subtitles`, `watch`, `watchlist`, `profiles`, `recommend`, `languages`, `disk`
and `letterboxd_import`. `remove` is one call that clears the catalog row, the library folder
and the torrent together — with hardlinks, doing only one of the three frees no
disk space. `remove` with `dry_run` lists what it would delete and touches nothing.

`find` is the MCP form of `butaca find`, for a film not in the catalog. It returns
JSON, and every release carries an `id` (the infohash, or the Prowlarr GUID) that
stays valid across searches. `find` with `grab: <id>` catalogues the film and downloads
that exact release. It fails, rather than picking another, if the release has left
the results. That split lets a client show the choices and ask before anything is
downloaded.

## Usage

```sh
butaca tui                                  # five tabs: library, watchlist, queue, discover, profiles
butaca find "Dune Part Two 2024"            # search for something you don't have
butaca find "Dune Part Two 2024" --grab 1   # add it and start the download
butaca add "Taxi Driver" --year 1976 --lang en   # add, search, grab; --lang only without a TMDB key
butaca add "Severance" --series             # series need a TMDB key
butaca add "Amélie" --lang fr --alt-title "Le Fabuleux Destin d'Amélie Poulain"
butaca search Amélie --explain              # every release, scored, with reasons
butaca search Amélie --grab                 # send the winner to qBittorrent
butaca import --watch                       # import finished downloads
butaca list
butaca rm "Taxi Driver"                     # catalog + disk + torrent
butaca removals                             # audit trail of what was deleted
butaca status                               # dependency health and queue
butaca config set paths.movies /media/movies
butaca setup                                # re-run the wizard
butaca migrate                              # import a Radarr catalog
butaca orphans                              # unaccounted-for downloads
```

`find` is the one to reach for when a film is not in the catalog yet: it needs
no TMDB key, because the release name supplies the year. `add` is for when you
already know exactly what you want catalogued.

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
  documentaries: /media/documentaries   # optional second movie library
  tv: /media/tv
  downloads: /media/downloads   # must share a filesystem with the others
prowlarr:
  url: http://192.168.1.10:9696 # routable, not localhost, if qBittorrent is a container
  api_key: ...
qbittorrent:
  url: http://localhost:8080
  category: butaca              # every torrent butaca adds carries this
tmdb:
  api_key: ""                   # optional: series, recommendations, automatic original_language
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

### Documentaries

With `paths.documentaries` set, documentaries go to their own folder, so Jellyfin
or Plex can show them as a separate library. They are still movies to butaca. The
only difference is which folder they live in, and that folder is the only record
of it. Nothing else needs to stay in sync.

```sh
butaca add "The Social Dilemma" --year 2020 --lang en --documentary
butaca find "Thinking Game 2024" --grab 1 --documentary
butaca move "Social Dilemma" movies --dry-run   # shows from and to
butaca move "Social Dilemma" documentaries
```

With a TMDB key, a film TMDB tags as Documentary goes there automatically;
`--documentary=false` overrides that. `move` renames the folder, so the hardlink
to the download and the subtitles next to the video go with it. Over MCP, the
same thing is `documentary` on `add` and `find`, plus the `move` tool.

## Profiles

A profile is a person, not a quality preset: their release preferences, their
subtitle languages, their Letterboxd account and their own watchlist.

```sh
butaca profile set Leandro --letterboxd leandroacosta \
  --language-mode original --subtitles es --resolutions 1080p --default
butaca profile set Casa --language-mode prefer --prefer-language es \
  --resolutions 1080p,720p
butaca profile assign Casa "Toy Story 5"
```

A profile overrides only the fields it sets; everything else falls back to the
global configuration.

## Watchlist

```sh
butaca watchlist              # films queued for some day
butaca watch "Vivarium"       # promote it so butaca searches for it
butaca unwatch "Toy Story 5"  # stop searching, keep the files
```

Watchlist entries are catalogued but **never searched**. That separation is what
lets a two-hundred-film import stay an intention instead of two hundred
downloads.

## Letterboxd

```sh
butaca letterboxd lists --user yourname
butaca letterboxd import --user yourname --list pelis-para-ver
```

Letterboxd has no public API. The member RSS feed carries lists but omits TMDB
ids, and the watchlist and ratings feeds answer 403, so this reads the public
HTML pages and resolves each film through `/film/{slug}/`, where the TMDB id
lives. It is a scraper, isolated in `internal/letterboxd` so that when the
markup changes one package fails loudly. Imports land on the watchlist and
download nothing.

## Recommendations

```sh
butaca recommend --limit 20
butaca recommend --add 3        # put suggestion 3 on the watchlist
```

Seeded by the films you actually own, not by Letterboxd ratings: a library
always has something to reason from, whereas an account with no rated films
produces nothing. Needs a TMDB API key. A film reached from several of your
titles outranks one reached from a single popular seed, and nothing already in
the catalog is ever suggested back.

Films added before a TMDB key was configured have no TMDB id, so they seed
nothing. Adding the key does not fix them on its own; run:

```sh
butaca tmdb-match            # fill TMDB id, language and alternative titles
butaca tmdb-match --apply    # also move films TMDB files under Documentary
```

It matches on title (or original title) within a year of the catalogued one,
since release years differ by country. When several films share the title, it
picks one only when that film is evident, and lists the candidates otherwise.
Titles, paths and languages set by hand are never changed. Films that already
have an id are skipped, so it is safe to run again.

## Languages

```sh
butaca scan-tracks     # probe every file with ffprobe
butaca languages       # what the library actually holds
butaca disk            # free space and what the library occupies
```

Track data comes from the files themselves, not their names. One release here
carries 10 audio and 44 subtitle tracks, which is why lists show `es fr en +7`
and the detail view shows all of them.

## Subtitle sync

A subtitle is found by scanning the video file itself, not just its name. That
computes the OpenSubtitles hash, and a hash match is a subtitle made for this
exact release, so it is in sync already. The release name from before the import
is passed along too, for scoring. Languages the file already carries as an
embedded track are not downloaded.

Anything that is not a hash match is synced right after download. It is synced
against an embedded text subtitle when the file has one (in any language, English
first), otherwise against the audio, with ffsubsync. If ffsubsync fails or does
not trust its result, alass tries next, since it copes with subtitles cut
differently. A shift of more than 60 seconds is rejected as a false match, and
the subtitle is left as it was.

```sh
butaca subtitles "Ghost in the Shell" --sync   # fix the subtitles already on disk
butaca subtitles --sync --all                  # the whole library
butaca subtitles --sync --all --force          # redo, starting from the originals
```

`--sync` first looks for a hash match to replace the subtitle with. Only if there
is none does it sync the one already there. The original is kept as
`<name>.<lang>.srt.orig`. The name deliberately does not end in `.srt`, so Jellyfin
does not list it as a second, out-of-sync track. A subtitle with an original next
to it counts as done and is skipped next time. Every outcome goes to the history.
Over MCP, the same thing is `subtitles` with `sync: true`.

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
