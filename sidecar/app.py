"""butaca-parse — the Python half of butaca.

Go owns state, concurrency and interfaces. This service owns the libraries
that are not worth porting: guessit (release-name parsing), subliminal
(subtitle search), and ffsubsync plus alass (subtitle sync). Kept deliberately
small: the subtitle logic lives in subfetch.py and subsync.py.
"""

from pathlib import Path
from typing import Any

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field

from babelfish import Language
from guessit import guessit

from media import ProbeError, probe
from subfetch import FetchResult, fetch, resync
from subsync import SyncOutcome

app = FastAPI(title="butaca-parse", version="1.0.0")


class ParseRequest(BaseModel):
    # Batch by design: one search returns ~200 releases and spawning a process
    # per title would dominate the request.
    titles: list[str] = Field(default_factory=list)


class ParseResult(BaseModel):
    title: str | None = None
    year: int | None = None
    language: str | None = None
    subtitle_language: str | None = None
    screen_size: str | None = None
    source: str | None = None
    video_codec: str | None = None
    audio_codec: str | None = None
    audio_channels: str | None = None
    release_group: str | None = None
    season: int | None = None
    episode: int | None = None
    type: str | None = None
    raw: str


def _one(value: Any) -> Any:
    """guessit returns a list when a field matched more than once."""
    if isinstance(value, list):
        return value[0] if value else None
    return value


def _text(value: Any) -> str | None:
    value = _one(value)
    if value is None:
        return None
    return str(value)


def _parse(raw: str) -> ParseResult:
    g = guessit(raw)
    return ParseResult(
        title=_text(g.get("title")),
        year=_one(g.get("year")),
        language=_text(g.get("language")),
        subtitle_language=_text(g.get("subtitle_language")),
        screen_size=_text(g.get("screen_size")),
        source=_text(g.get("source")),
        video_codec=_text(g.get("video_codec")),
        audio_codec=_text(g.get("audio_codec")),
        audio_channels=_text(g.get("audio_channels")),
        release_group=_text(g.get("release_group")),
        season=_one(g.get("season")),
        episode=_one(g.get("episode")),
        type=_text(g.get("type")),
        raw=raw,
    )


@app.get("/health")
def health() -> dict[str, str]:
    return {"status": "ok"}


@app.post("/parse", response_model=list[ParseResult])
def parse(req: ParseRequest) -> list[ParseResult]:
    return [_parse(t) for t in req.titles]


class SubtitleRequest(BaseModel):
    path: str
    languages: list[str] = Field(default_factory=lambda: ["es"])
    # The name the file was downloaded under, before the import renamed it.
    release_name: str | None = None
    sync: bool = True


def _video(path: str) -> Path:
    video = Path(path)
    if not video.is_file():
        raise HTTPException(status_code=404, detail=f"no such file: {path}")
    return video


def _check_languages(codes: list[str]) -> None:
    try:
        for code in codes:
            Language.fromietf(code)
    except Exception as exc:
        raise HTTPException(status_code=400, detail=f"bad language code: {exc}") from exc


@app.post("/subtitles", response_model=FetchResult)
def subtitles(req: SubtitleRequest) -> FetchResult:
    """Fetch the missing languages; sync whatever is not a hash match."""
    video = _video(req.path)
    _check_languages(req.languages)
    return fetch(video, req.languages, req.release_name, req.sync)


class SyncRequest(BaseModel):
    path: str
    lang: str = "es"
    release_name: str | None = None
    # Re-sync from the kept original even if this subtitle was synced before.
    force: bool = False


@app.post("/sync", response_model=SyncOutcome)
def sync(req: SyncRequest) -> SyncOutcome:
    """Fix the subtitle already next to a video: replace it with a hash match
    when one exists, otherwise sync it in place. The original is kept."""
    video = _video(req.path)
    _check_languages([req.lang])
    return resync(video, req.lang, req.release_name, req.force)


class TracksRequest(BaseModel):
    path: str


class Track(BaseModel):
    kind: str  # audio | subtitle
    lang: str
    title: str | None = None
    codec: str | None = None


class TracksResult(BaseModel):
    tracks: list[Track] = Field(default_factory=list)


@app.post("/tracks", response_model=TracksResult)
def tracks(req: TracksRequest) -> TracksResult:
    """Report the audio and subtitle streams inside a media file.

    Answers "which languages do I actually have?" -- a question neither the
    release name nor the catalog can answer, because a single file can carry
    ten audio tracks and forty subtitle tracks.
    """
    video = _video(req.path)
    try:
        streams = probe(video)
    except ProbeError as exc:
        raise HTTPException(status_code=422, detail=str(exc)) from exc
    # An untagged stream is common in older rips; "und" keeps it countable
    # instead of silently dropping a track that exists.
    return TracksResult(tracks=[
        Track(kind=s.kind, lang=s.lang, title=s.title, codec=s.codec)
        for s in streams if s.kind in ("audio", "subtitle")
    ])
