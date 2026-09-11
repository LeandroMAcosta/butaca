"""Finding subtitles with subliminal, scored against the actual file.

The video is scanned from disk, not built from its name: that computes the
file hashes, and a hash match is a subtitle made for this exact release, in
sync by construction. The library file is already renamed to "Title (Year)",
so the original release name is merged back in for release-group, source and
resolution scoring.
"""

import logging
from pathlib import Path

from babelfish import Language
from guessit import guessit
from pydantic import BaseModel, Field
from subliminal import (Video, compute_score, download_best_subtitles, download_subtitles,
                        list_subtitles, refine, region, save_subtitles, scan_video)

from media import embedded_subtitle_languages, probe
from subsync import SyncOutcome, keep_original, original_of, sync_subtitle

log = logging.getLogger("butaca.subfetch")

if not region.is_configured:
    region.configure("dogpile.cache.memory")

# Attributes the release name knows and the renamed file has lost.
_RELEASE_ATTRS = ("source", "release_group", "resolution", "video_codec", "audio_codec",
                  "streaming_service")


class Fetched(BaseModel):
    lang: str
    path: str
    provider: str
    score: int
    hash_match: bool
    sync: SyncOutcome | None = None


class FetchResult(BaseModel):
    downloaded: list[str] = Field(default_factory=list)
    skipped: list[str] = Field(default_factory=list)
    embedded: list[str] = Field(default_factory=list)
    results: list[Fetched] = Field(default_factory=list)


def scan(path: Path, release_name: str | None) -> Video:
    video = scan_video(str(path))
    refine(video, movie_refiners=("hash", "metadata"), episode_refiners=("hash", "metadata"),
           embedded_subtitles=False)
    if release_name:
        try:
            named = Video.fromguess(release_name, guessit(release_name))
        except Exception as exc:  # an unparsable name only costs some score
            log.info("release name %r not usable: %s", release_name, exc)
        else:
            for attr in _RELEASE_ATTRS:
                if not getattr(video, attr, None) and getattr(named, attr, None):
                    setattr(video, attr, getattr(named, attr))
    return video


def fetch(path: Path, languages: list[str], release_name: str | None, sync: bool) -> FetchResult:
    """Download the best subtitle per missing language, then sync it unless
    it is a hash match. Languages already embedded in the file are skipped."""
    result = FetchResult()
    try:
        embedded = embedded_subtitle_languages(probe(path))
    except Exception as exc:
        log.warning("probe %s: %s", path, exc)
        embedded = set()
    wanted = [c for c in languages if c not in embedded]
    result.embedded = [c for c in languages if c in embedded]
    if not wanted:
        result.skipped = list(languages)
        return result

    video = scan(path, release_name)
    video.subtitle_languages = {Language.fromietf(c) for c in embedded}
    found = download_best_subtitles([video], {Language.fromietf(c) for c in wanted})
    for sub in save_subtitles(video, found[video]):
        dest = Path(sub.get_path(video))
        # A fresh download replaces whatever was there, so an original kept
        # from an earlier sync belongs to a different file now.
        original_of(dest).unlink(missing_ok=True)
        hit = "hash" in sub.get_matches(video)
        item = Fetched(lang=sub.language.alpha2, path=str(dest), provider=sub.provider_name,
                       score=compute_score(sub, video), hash_match=hit)
        if sync and not hit:
            item.sync = sync_subtitle(path, dest)
        result.results.append(item)
        result.downloaded.append(str(dest))

    got = {r.lang for r in result.results}
    result.skipped = [c for c in languages if c not in got]
    return result


def resync(path: Path, lang: str, release_name: str | None, force: bool) -> SyncOutcome:
    """Fix the subtitle already next to a video.

    A hash-matched subtitle replaces it when one exists; otherwise the one
    there is synced in place. Either way the original is kept.
    """
    try:
        embedded = embedded_subtitle_languages(probe(path))
    except Exception:
        embedded = set()
    if lang in embedded:
        return SyncOutcome(status="skipped", detail=f"the file already has an embedded {lang} track")

    subtitle = path.with_suffix(f".{lang}.srt")
    if original_of(subtitle).exists() and not force:
        return SyncOutcome(status="skipped", detail=f"already synced; original at {original_of(subtitle).name}")

    video = scan(path, release_name)
    language = Language.fromietf(lang)
    hit = _hash_match(video, language)
    if hit is not None:
        keep_original(subtitle)
        save_subtitles(video, [hit])
        return SyncOutcome(status="refetched", method="hash", score=compute_score(hit, video),
                           detail=f"hash match from {hit.provider_name}")

    if not subtitle.is_file():
        fetched = fetch(path, [lang], release_name, sync=True)
        if not fetched.results:
            return SyncOutcome(status="failed", detail=f"no {lang} subtitle found")
        return fetched.results[0].sync or SyncOutcome(status="refetched", method="hash")
    return sync_subtitle(path, subtitle, force=force)


def _hash_match(video: Video, language: Language):
    """The best downloadable subtitle made for this exact file, if any."""
    listed = list_subtitles({video}, {language}).get(video, [])
    hits = [s for s in listed if "hash" in s.get_matches(video)]
    hits.sort(key=lambda s: compute_score(s, video), reverse=True)
    for sub in hits[:5]:
        download_subtitles([sub])
        if sub.content and sub.is_valid():
            return sub
    return None
