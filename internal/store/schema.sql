PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

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

CREATE INDEX IF NOT EXISTS idx_files_item   ON files(item_id);
CREATE INDEX IF NOT EXISTS idx_queue_state  ON queue(state);
CREATE INDEX IF NOT EXISTS idx_history_item ON history(item_id);
