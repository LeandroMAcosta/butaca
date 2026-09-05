PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

-- A profile is a person, not a quality preset: their release preferences, their
-- Letterboxd account, and (through items.profile_id) their own watchlist. An
-- item without a profile falls back to the config defaults, so a fresh install
-- needs no profiles at all.
CREATE TABLE IF NOT EXISTS profiles (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    name            TEXT    NOT NULL UNIQUE,
    letterboxd_user TEXT,
    resolutions     TEXT,
    sources         TEXT,
    min_size        TEXT,
    max_size        TEXT,
    min_seeders     INTEGER NOT NULL DEFAULT 0,
    language_mode   TEXT    NOT NULL DEFAULT 'original',
    prefer_language TEXT,
    subtitle_langs  TEXT,
    is_default      INTEGER NOT NULL DEFAULT 0
);

-- One table for movies and series. kind discriminates; the import path is shared.
CREATE TABLE IF NOT EXISTS items (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    kind              TEXT    NOT NULL CHECK (kind IN ('movie','series')),
    tmdb_id           INTEGER,
    imdb_id           TEXT,
    title             TEXT    NOT NULL,
    year              INTEGER,
    original_language TEXT,
    alt_titles        TEXT,
    path              TEXT,
    monitored         INTEGER NOT NULL DEFAULT 1,
    -- state separates "go get this" from "some day": watchlist entries are
    -- catalogued but never searched until promoted.
    state             TEXT    NOT NULL DEFAULT 'monitored',
    profile_id        INTEGER REFERENCES profiles(id) ON DELETE SET NULL,
    rating            REAL,
    source            TEXT,
    added_at          TEXT    NOT NULL DEFAULT (datetime('now')),
    UNIQUE (kind, tmdb_id)
);

CREATE TABLE IF NOT EXISTS seasons (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    item_id   INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    number    INTEGER NOT NULL,
    monitored INTEGER NOT NULL DEFAULT 1,
    UNIQUE (item_id, number)
);

CREATE TABLE IF NOT EXISTS episodes (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    season_id INTEGER NOT NULL REFERENCES seasons(id) ON DELETE CASCADE,
    number    INTEGER NOT NULL,
    title     TEXT,
    air_date  TEXT,
    monitored INTEGER NOT NULL DEFAULT 1,
    UNIQUE (season_id, number)
);

-- episode_id NULL means the file belongs to a movie.
CREATE TABLE IF NOT EXISTS files (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    item_id       INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    episode_id    INTEGER REFERENCES episodes(id) ON DELETE CASCADE,
    path          TEXT    NOT NULL UNIQUE,
    size          INTEGER NOT NULL DEFAULT 0,
    quality       TEXT,
    source        TEXT,
    video_codec   TEXT,
    audio_codec   TEXT,
    languages     TEXT,
    release_group TEXT,
    added_at      TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS subtitles (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    file_id    INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    lang       TEXT    NOT NULL,
    path       TEXT    NOT NULL,
    source     TEXT,
    fetched_at TEXT    NOT NULL DEFAULT (datetime('now')),
    UNIQUE (file_id, lang)
);

CREATE TABLE IF NOT EXISTS queue (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    item_id       INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    episode_id    INTEGER REFERENCES episodes(id) ON DELETE CASCADE,
    release_title TEXT    NOT NULL,
    magnet        TEXT,
    info_hash     TEXT    NOT NULL,
    state         TEXT    NOT NULL DEFAULT 'grabbed',
    size          INTEGER NOT NULL DEFAULT 0,
    progress      REAL    NOT NULL DEFAULT 0,
    grabbed_at    TEXT    NOT NULL DEFAULT (datetime('now')),
    UNIQUE (info_hash)
);

CREATE TABLE IF NOT EXISTS history (
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    item_id INTEGER REFERENCES items(id) ON DELETE SET NULL,
    event   TEXT    NOT NULL,
    detail  TEXT,
    at      TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- Audio and subtitle tracks found inside a file, so the library can answer
-- "which languages do I actually have?" without re-probing every time.
CREATE TABLE IF NOT EXISTS tracks (
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    kind    TEXT    NOT NULL CHECK (kind IN ('audio','subtitle')),
    lang    TEXT    NOT NULL,
    title   TEXT,
    UNIQUE (file_id, kind, lang, title)
);

CREATE INDEX IF NOT EXISTS idx_files_item   ON files(item_id);
CREATE INDEX IF NOT EXISTS idx_tracks_file  ON tracks(file_id);

-- Resolving a Letterboxd slug to a TMDB id costs one request and never
-- changes, so it is cached permanently.
CREATE TABLE IF NOT EXISTS letterboxd_films (
    slug     TEXT PRIMARY KEY,
    tmdb_id  INTEGER,
    title    TEXT,
    year     INTEGER,
    fetched_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_queue_state  ON queue(state);
CREATE INDEX IF NOT EXISTS idx_history_item ON history(item_id);
