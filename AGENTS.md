# AGENTS.md

Working notes for anyone — human or agent — changing this codebase.

## What butaca is

One service that replaces Radarr, Sonarr and Bazarr: catalog, release selection,
import and subtitles for movies and series. It keeps two external dependencies
because their value is not worth reimplementing:

- **Prowlarr** (`:9696`) — one search API in front of every tracker
- **qBittorrent** (`:8080`) — downloading

And one sidecar of its own:

- **butaca-parse** (`:8000`) — a small FastAPI service owning `guessit`
  (release-name parsing), `subliminal` (subtitles), `ffsubsync` and `alass`
  (subtitle sync) and `ffprobe` (audio and subtitle tracks). Go owns state,
  concurrency and interfaces; Python owns the libraries not worth porting.

## Layout

```
cmd/butaca/            cobra CLI, one file per command group
internal/
  app/                 orchestration; everything the CLI, TUI and MCP share
  config/              YAML + BUTACA_* env overrides
  store/               SQLite; one file per table group
  decide/              release scoring  ← the reason this project exists
  indexer/             Prowlarr client
  download/            qBittorrent client
  parse/               butaca-parse client
  metadata/            TMDB (movies, TV, recommendations)
  library/             import, hardlinks, disk usage
  letterboxd/          HTML scraper, deliberately isolated
  recommend/           ranking of TMDB suggestions
  scheduler/           background jobs for `serve`
  mcpserver/           15 MCP tools
  tui/                 bubbletea: setup wizard + five-tab browser
  migrate/             read Radarr/Sonarr databases
sidecar/               butaca-parse
```

## Invariants worth knowing before you change anything

**Imports are hardlinks, never copies.** A file seeds under its download name
and appears in the library under a clean one, as one set of bytes with two
names. Two consequences that have already caused bugs:

- Deleting one name frees *nothing*. `app.Remove` tears down the catalog row,
  the library folder and the torrent together; keep it that way.
- `du` attributes the bytes to whichever directory it walks first, which is why
  `~/Movies` can report kilobytes while holding 100 GB.
- `paths.downloads` must share a filesystem with `paths.movies` and `paths.tv`.
  `library.SameFilesystem` checks this and `status` reports it. Never fall back
  to copying: it silently doubles disk usage.

**Deletions are audited before they happen.** `app.Remove` writes a
`remove_requested` row to `history` *before* touching anything, and
`history.item_id` is `ON DELETE SET NULL`, so the record survives the row it
describes. `butaca removals` reads it. This exists because four films once
vanished from the catalog and the library with no trace of what removed them —
the bytes survived in `~/Downloads` only because imports are hardlinks.

**`items.state` separates intent from action.** `watchlist` entries are
catalogued but never searched; `SearchMissing` skips them. This is the only
thing stopping a two-hundred-film Letterboxd import from starting two hundred
downloads. Re-importing never overwrites a state the user already changed.

**Language mode is relative to the film.** `LangOriginal` compares a release
against TMDB's `original_language`, not a constant. The stack this replaced had
a fixed English filter, which rejected Amélie, Exit 8 and Christiane F. outright
and forced manual grabs. Do not reintroduce a global "wanted language".

**A profile is a person, not a quality preset.** It carries preferences, subtitle
languages, a Letterboxd account and (via `items.profile_id`) its own watchlist.
`applyProfile` overrides only the fields the profile actually sets.

## Gotchas found the hard way

- **guessit returns ISO codes from the library** (`ja`, `de`) but English names
  from its CLI. `decide.languageMatches` handles both directions.
- **`subtitle_language` is not `language`.** "Amelie 2001 English subs" declares
  subtitles, not audio. Reading one as the other rejects good releases.
- **`mul` and `und` mean "unknown", not "wrong".** A multi-audio release usually
  includes the original track.
- **Title matching must not be substring containment.** "Dawn of the Deep Soul"
  contains "Soul". Matching is equality-or-prefix against every known title,
  which is why `alt_titles` (TMDB's `original_title`) exists.
- **Prowlarr passes through HTML entities** from some trackers (`Am&eacute;lie`).
  The indexer client unescapes them.
- **A language bonus must not outweigh swarm health.** It is a tiebreaker
  (`langMatchBonus = 15`); a release that merely names the right language once
  beat one with 15× the seeders.
- **`ON CONFLICT(kind, tmdb_id)` collapses everything without a TMDB key**,
  because every row gets `tmdb_id = 0`. Rows with no TMDB id store NULL and key
  on `(kind, title, year)` instead.
- **Indexes on migrated columns go in `store.Open` after the ALTERs**, not in
  `schema.sql`: an old database cannot index a column that does not exist yet.
- **Never query the database from a TUI view.** Views repaint on every keystroke.
  Load into the model in `reload()`; `b.langs` exists for exactly this reason.
- **qBittorrent inside Docker Desktop on macOS kills every container's egress.**
  The VM engine spends one host thread per guest flow; DHT opens thousands and
  macOS caps a process at 4096. `status` cannot see it because it only checks
  that Prowlarr answers, not that its indexers can. Run qBittorrent natively.
- **subliminal's `scan_video` does not compute file hashes.** Without
  `refine(video, movie_refiners=("hash", ...))` no subtitle can hash-match, and
  `Video.fromname` has neither hashes nor the release name the import removed.
- **ffsubsync and alass are picky about their input.** Both choose the parser by
  extension (a `.srt.orig` fails), alass reads only UTF-8, and ffsubsync logs
  through rich, which wraps at 80 columns off a terminal. `subsync.py` copies
  the subtitle to a UTF-8 `in.srt` and sets `COLUMNS` before parsing the log.
- **TUI commands guard against a nil `app`**, which is what makes the whole
  interface testable headlessly.

## Conventions

- Code, comments and identifiers in English. User-facing strings too — this is a
  developer tool with an English CLI.
- **Files stay under 300 lines.** Split by responsibility when they grow.
- `gofmt` and `go vet` clean before every commit.
- Comments explain *why*, not what. Prefer one sentence about a non-obvious
  constraint over a paragraph restating the code.
- Errors say what failed and how to fix it: `"no TMDB API key: pass --lang ..."`.
- No secrets in the repo. `config.yaml`, `.env` and `*.db` are gitignored.

## Running and testing

```sh
go build -o butaca ./cmd/butaca
go test ./...                          # unit; no network, no database
BUTACA_IT=1 go test ./internal/app/    # against a live qBittorrent
BUTACA_LETTERBOXD_USER=name go test ./internal/letterboxd/ -run Live
```

The sidecar, for local work:

```sh
cd sidecar && uv run --with fastapi --with 'uvicorn[standard]' \
  --with guessit --with subliminal --with babelfish \
  uvicorn app:app --port 8000
```

Everything together: `docker compose up` (needs `PROWLARR_API_KEY` in `.env`).

Decision-engine tests use **real release names captured from Prowlarr**,
including the three films the previous stack rejected. When you change scoring,
those tests are the specification.

## Known limits

- **Recommendations and series need a TMDB API key.** The ranking logic is unit
  tested; the live path is not, because no key was available.
- **Letterboxd is scraping.** RSS carries lists without TMDB ids; watchlist and
  ratings feeds answer 403. It reads public HTML with a browser user agent and
  will break when the markup changes — by design, loudly and in one package.
- Season packs are rejected for single-episode searches; they need different
  import handling.
- No HTTP API. Interfaces are CLI, TUI and MCP.
- Release lists deduplicate before display: Prowlarr fans one query across every
  tracker, so a popular film returns ~200 rows of which most are the same
  release twice and the rest are rejects. `internal/tui/candidates.go` merges by
  normalised title, keeps the best-seeded copy, shows one line each, and hides
  rejects behind `x`. `cmd/butaca/find_cmd.go` does the same for the CLI.
- `find` / the TUI's `a` key work without TMDB by matching the query against
  release names; the item gets no `original_language`, which the engine reads as
  "no language constraint" rather than rejecting everything.
