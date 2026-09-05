"""butaca-parse — the Python half of butaca.

Go owns state, concurrency and interfaces. This service owns the two libraries
that are not worth porting: guessit (release-name parsing) and subliminal
(subtitle search). Kept deliberately small.
"""

from pathlib import Path
from typing import Any

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field

from babelfish import Language
from guessit import guessit
from subliminal import Video, download_best_subtitles, save_subtitles

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


class SubtitleResult(BaseModel):
    downloaded: list[str] = Field(default_factory=list)
    skipped: list[str] = Field(default_factory=list)


@app.post("/subtitles", response_model=SubtitleResult)
def subtitles(req: SubtitleRequest) -> SubtitleResult:
    video_path = Path(req.path)
    if not video_path.is_file():
        raise HTTPException(status_code=404, detail=f"no such file: {req.path}")

    try:
        langs = {Language.fromietf(code) for code in req.languages}
    except Exception as exc:
        raise HTTPException(status_code=400, detail=f"bad language code: {exc}") from exc

    video = Video.fromname(str(video_path))
    # Embedded tracks count as present; subliminal skips languages already there.
    found = download_best_subtitles([video], langs)
    saved = save_subtitles(video, found[video])

    downloaded = [str(video_path.with_suffix("")) + f".{s.language.alpha2}.srt" for s in saved]
    got = {s.language.alpha2 for s in saved}
    skipped = [c for c in req.languages if c not in got]
    return SubtitleResult(downloaded=downloaded, skipped=skipped)
