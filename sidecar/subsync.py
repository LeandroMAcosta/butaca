"""Subtitle synchronisation: ffsubsync first, alass as the fallback.

ffsubsync lines up speech in a reference (an embedded text subtitle, or the
audio) with the subtitle's on-screen intervals and finds the best offset and
framerate ratio. alass handles what a single offset cannot: a subtitle made for
a different cut, with splits. Either result is thrown away if the shift is
implausible, and the original subtitle is always kept next to the synced one.
"""

import logging
import os
import re
import shutil
import statistics
import subprocess
import sys
import tempfile
from pathlib import Path

import srt
from pydantic import BaseModel, Field

from media import Stream, probe, text_reference

log = logging.getLogger("butaca.subsync")

# Beyond this the "sync" is almost certainly a false match, not a fix.
MAX_OFFSET_SECONDS = 60.0
ALASS_SPLIT_PENALTY = "7"
TIMEOUT_SECONDS = 900

# "Movie.es.srt" keeps its original as "Movie.es.srt.orig". Not ".orig.srt": a
# name ending in .srt shows up in Jellyfin as a second, out-of-sync track.
ORIG_SUFFIX = ".orig"


class SyncOutcome(BaseModel):
    status: str  # synced | refetched | rejected | failed | skipped
    method: str | None = None  # hash | embedded | audio | alass-embedded | alass-audio
    offset_seconds: float | None = None
    framerate_scale: float | None = None
    score: float | None = None
    detail: str | None = None
    attempts: list[str] = Field(default_factory=list)


def original_of(subtitle: Path) -> Path:
    return subtitle.with_name(subtitle.name + ORIG_SUFFIX)


def keep_original(subtitle: Path) -> None:
    """Save the subtitle as it was, once. Later runs never overwrite it."""
    orig = original_of(subtitle)
    if subtitle.is_file() and not orig.exists():
        shutil.copy2(subtitle, orig)


def alass_binary() -> str | None:
    return os.environ.get("ALASS_PATH") or shutil.which("alass-cli") or shutil.which("alass")


def sync_subtitle(video: Path, subtitle: Path, force: bool = False) -> SyncOutcome:
    """Sync subtitle in place against video, keeping the original.

    Already-synced subtitles (their original is kept) are skipped unless force
    is set, in which case the original, not the previous result, is re-synced.
    """
    orig = original_of(subtitle)
    if orig.exists() and not force:
        return SyncOutcome(status="skipped", detail=f"already synced; original at {orig.name}")
    source = orig if orig.exists() else subtitle
    if not source.is_file():
        return SyncOutcome(status="failed", detail=f"no subtitle at {subtitle}")

    try:
        ref = text_reference(probe(video))
    except Exception as exc:  # a probe failure still leaves the audio to try
        log.warning("probe %s: %s", video, exc)
        ref = None

    attempts: list[str] = []
    rejected = False
    with tempfile.TemporaryDirectory(prefix=".butaca-sync-", dir=subtitle.parent) as tmp:
        out = Path(tmp) / "out.srt"
        # Both tools pick the parser by extension, and ".orig" is not one; and
        # alass reads only UTF-8, while many Spanish subtitles are Latin-1.
        work = Path(tmp) / "in.srt"
        work.write_text(_read(source), encoding="utf-8")
        source = work
        plans = []
        if ref is not None:
            plans.append(("embedded", lambda: _ffsubsync(video, source, out, ref)))
        plans.append(("audio", lambda: _ffsubsync(video, source, out, None)))
        plans.append((
            "alass-embedded" if ref is not None else "alass-audio",
            lambda: _alass(video, source, out, ref, Path(tmp)),
        ))

        for method, attempt in plans:
            out.unlink(missing_ok=True)
            result = attempt()
            attempts.append(f"{method}: {result.detail}")
            log.info("sync %s via %s: %s", subtitle.name, method, result.detail)
            if result.status == "synced":
                keep_original(subtitle)
                os.replace(out, subtitle)
                result.method, result.attempts = method, attempts
                return result
            rejected = rejected or result.status == "rejected"

    return SyncOutcome(
        status="rejected" if rejected else "failed",
        detail="every method failed or was rejected; subtitle left as it was",
        attempts=attempts,
    )


_SCORE = re.compile(r"score: (-?[\d.]+)")
_OFFSET = re.compile(r"offset seconds: (-?[\d.]+)")
_SCALE = re.compile(r"framerate scale factor: (-?[\d.]+)")
_LOW = re.compile(r"low-quality alignment \((.*?)\);", re.DOTALL)


def _ffsubsync(video: Path, source: Path, out: Path, ref: Stream | None) -> SyncOutcome:
    # A subprocess, not the library in-process: a crash or a hang in ffmpeg
    # must not take the sidecar down, and the timeout must be enforceable.
    cmd = [
        sys.executable, "-c", "import sys; from ffsubsync.ffsubsync import main; sys.exit(main())",
        str(video), "-i", str(source), "-o", str(out),
        "--max-offset-seconds", str(int(MAX_OFFSET_SECONDS)),
        "--skip-sync-on-low-quality",
        "--quality-max-offset-seconds", str(int(MAX_OFFSET_SECONDS)),
    ]
    if ref is not None:
        cmd += ["--reference-stream", f"0:s:{ref.ordinal}"]
    try:
        # ffsubsync logs through rich, which wraps at 80 columns when not on a
        # terminal and would split the lines parsed below.
        proc = subprocess.run(cmd, capture_output=True, text=True, timeout=TIMEOUT_SECONDS,
                              env={**os.environ, "COLUMNS": "400"})
    except subprocess.TimeoutExpired:
        return SyncOutcome(status="failed", detail="ffsubsync timed out")
    logs = proc.stdout + proc.stderr

    score, offset, scale = _last(_SCORE, logs), _last(_OFFSET, logs), _last(_SCALE, logs)
    if "low-quality alignment" in logs:
        low = _LOW.search(logs)
        reason = " ".join(low.group(1).split()) if low else "ffsubsync did not trust it"
        return SyncOutcome(status="rejected", score=score, offset_seconds=offset,
                           framerate_scale=scale, detail=f"low quality: {reason}")
    if proc.returncode != 0 or offset is None or not out.is_file():
        tail = logs.strip().splitlines()[-1:] or ["no output"]
        return SyncOutcome(status="failed", detail=f"ffsubsync: {tail[0][:200]}")
    if abs(offset) > MAX_OFFSET_SECONDS:
        return SyncOutcome(status="rejected", offset_seconds=offset,
                           detail=f"offset {offset:.1f}s is implausible")
    return SyncOutcome(status="synced", score=score, offset_seconds=offset, framerate_scale=scale,
                       detail=f"offset {offset:+.2f}s, framerate x{scale or 1:.3f}, score {score}")


def _alass(video: Path, source: Path, out: Path, ref: Stream | None, tmp: Path) -> SyncOutcome:
    binary = alass_binary()
    if not binary:
        return SyncOutcome(status="failed", detail="alass is not installed")
    reference: Path = video
    if ref is not None:
        reference = tmp / "reference.srt"
        extracted = _extract_subtitle(video, ref, reference)
        if extracted:
            return SyncOutcome(status="failed", detail=extracted)
    try:
        proc = subprocess.run(
            [binary, "--split-penalty", ALASS_SPLIT_PENALTY, str(reference), str(source), str(out)],
            capture_output=True, text=True, timeout=TIMEOUT_SECONDS,
        )
    except subprocess.TimeoutExpired:
        return SyncOutcome(status="failed", detail="alass timed out")
    if proc.returncode != 0 or not out.is_file():
        tail = (proc.stderr or proc.stdout).strip().splitlines()[-1:] or ["no output"]
        return SyncOutcome(status="failed", detail=f"alass: {tail[0][:200]}")

    shifts = _shifts(source, out)
    if not shifts:
        return SyncOutcome(status="failed", detail="alass output could not be read")
    offset = statistics.median(shifts)
    spread = max(shifts) - min(shifts)
    if abs(offset) > MAX_OFFSET_SECONDS:
        return SyncOutcome(status="rejected", offset_seconds=offset,
                           detail=f"median shift {offset:.1f}s is implausible")
    return SyncOutcome(status="synced", offset_seconds=offset,
                       detail=f"median shift {offset:+.2f}s, spread {spread:.2f}s "
                              f"(split penalty {ALASS_SPLIT_PENALTY})")


def _extract_subtitle(video: Path, ref: Stream, dest: Path) -> str | None:
    ffmpeg = shutil.which("ffmpeg")
    if not ffmpeg:
        return "ffmpeg is not installed"
    try:
        subprocess.run(
            [ffmpeg, "-v", "error", "-y", "-i", str(video), "-map", f"0:s:{ref.ordinal}", "-f", "srt", str(dest)],
            capture_output=True, text=True, timeout=300, check=True,
        )
    except (subprocess.CalledProcessError, subprocess.TimeoutExpired) as exc:
        return f"could not extract subtitle track {ref.ordinal}: {exc}"
    return None


def _shifts(before: Path, after: Path) -> list[float]:
    """How far each cue moved, in seconds. alass keeps cue order and count."""
    try:
        a = list(srt.parse(_read(before)))
        b = list(srt.parse(_read(after)))
    except Exception:
        return []
    return [(y.start - x.start).total_seconds() for x, y in zip(a, b)]


def _read(path: Path) -> str:
    raw = path.read_bytes()
    for enc in ("utf-8-sig", "cp1252", "latin-1"):
        try:
            return raw.decode(enc)
        except UnicodeDecodeError:
            continue
    return raw.decode("utf-8", errors="replace")


def _last(pattern: re.Pattern[str], text: str) -> float | None:
    found = pattern.findall(text)
    return float(found[-1]) if found else None
