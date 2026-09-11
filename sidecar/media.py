"""What is inside a media file, read with ffprobe."""

import json
import shutil
import subprocess
from dataclasses import dataclass
from pathlib import Path

from babelfish import Language

# Subtitle codecs that are text, so ffmpeg can extract them as a timing
# reference. Image formats (PGS, VobSub) would need OCR.
TEXT_SUBTITLE_CODECS = {"subrip", "ass", "ssa", "mov_text", "webvtt", "text"}


class ProbeError(Exception):
    """ffprobe is missing, failed or timed out."""


@dataclass(frozen=True)
class Stream:
    kind: str  # audio | subtitle | video
    codec: str
    lang: str  # as tagged, usually ISO 639-2 ("spa"), "und" when untagged
    title: str | None
    ordinal: int  # position among streams of the same kind: ffmpeg's 0:s:N


def probe(path: Path) -> list[Stream]:
    ffprobe = shutil.which("ffprobe")
    if not ffprobe:
        raise ProbeError("ffprobe is not installed")
    try:
        out = subprocess.run(
            [ffprobe, "-v", "error", "-show_entries",
             "stream=index,codec_type,codec_name:stream_tags=language,title",
             "-of", "json", str(path)],
            capture_output=True, text=True, timeout=120, check=True,
        ).stdout
    except subprocess.CalledProcessError as exc:
        raise ProbeError(f"ffprobe failed: {exc.stderr[:200]}") from exc
    except subprocess.TimeoutExpired as exc:
        raise ProbeError("ffprobe timed out") from exc

    streams: list[Stream] = []
    seen: dict[str, int] = {}
    for s in json.loads(out).get("streams", []):
        kind = s.get("codec_type") or ""
        ordinal = seen.get(kind, 0)
        seen[kind] = ordinal + 1
        tags = s.get("tags") or {}
        streams.append(Stream(
            kind=kind,
            codec=s.get("codec_name") or "",
            lang=tags.get("language") or "und",
            title=tags.get("title"),
            ordinal=ordinal,
        ))
    return streams


def alpha2(code: str) -> str | None:
    """"spa" or "es" -> "es"; None for "und" and anything unknown."""
    code = code.strip().lower()
    for convert in (Language.fromalpha3b, Language.fromalpha3t, Language.fromietf):
        try:
            return convert(code).alpha2
        except Exception:
            continue
    return None


def embedded_subtitle_languages(streams: list[Stream]) -> set[str]:
    """Languages already inside the file, as ISO 639-1 codes."""
    return {a for s in streams if s.kind == "subtitle" and (a := alpha2(s.lang))}


def text_reference(streams: list[Stream]) -> Stream | None:
    """The embedded text subtitle to sync against, English first.

    Any language works as a timing reference; English is simply the track
    most often complete in a release.
    """
    text = [s for s in streams if s.kind == "subtitle" and s.codec in TEXT_SUBTITLE_CODECS]
    for s in text:
        if alpha2(s.lang) == "en":
            return s
    return text[0] if text else None
