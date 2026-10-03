"""Durable per-cue synthesis progress stored inside the translations artifact.

`synthesize` saves `translated-asr.json` only after every cue succeeds, so one
genuine failure discards the speech evidence of the cues that already fit. This
module owns the narrow reuse contract: it records one verified checkpoint per
fitted cue inside that same artifact and re-accepts it only while every bound
input still matches. That includes a stamp taken when the cue actually fitted:
the accepted translations document is editable, so comparing a record only with
itself would happily reuse audio that no longer speaks the accepted text. It
never invokes a model. Regeneration stays the owner's decision, and an
unprovable model identity simply disables reuse instead of claiming a cache hit
it cannot support.
"""

from __future__ import annotations

import hashlib
import json
import os
import shutil
import subprocess
import tempfile
from pathlib import Path

import dub_contract
import dub_media

CHECKPOINT_KEY = "checkpoint"
CHECKPOINT_VERSION = 1
# Explicit stable policy facts, not a snapshot of whatever cap happens to be
# compiled in: a change to any of them must invalidate every recorded cue.
POLICY_IDENTIFIER = "dub-video/reference-conditioned-tts/1"
MAXIMUM_SPEECH_SPEED = 2.0
NONVERBAL_POLICY = "source-interval-speed-1"
INSPECT_TIMEOUT_SECONDS = 120.0
OPERATOR_CONFIG_PARTS = (".you-agent-factory", "config.json")
SAMPLES_PER_MILLISECOND = 24
BYTES_PER_SAMPLE = 2
REFERENCE_CONDITIONED = "reference-conditioned"
SOURCE_NONVERBAL = "source-nonverbal"
# The exact accepted input and presentation a cue was fitted from, plus the
# immutable fingerprint that binds them. Acceptance metadata alone is not the
# binding: the editable translations document is re-read on every stage, so the
# stamp taken at completion is compared against the accepted cue again.
CUE_INPUT_KEYS = ("id", "start", "end", "source_text", "text")
CUE_PRESENTATION_KEYS = ("audio_origin", "speech_end", "speech_speed", "nonverbal_kind")


def _canonical(value) -> str:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"))


def _fingerprint(value) -> str:
    return hashlib.sha256(_canonical(value).encode("utf-8")).hexdigest()


def file_sha256(path) -> str:
    """Local, streaming digest of one artifact or cache file."""
    value = hashlib.sha256()
    with Path(path).open("rb") as source:
        for chunk in iter(lambda: source.read(4 * 1024 * 1024), b""):
            value.update(chunk)
    return value.hexdigest()


def write_json(path: Path, value) -> None:
    """Atomic replace, so interrupted progress never truncates the artifact."""
    temporary = path.with_suffix(path.suffix + ".tmp")
    temporary.write_text(json.dumps(value, ensure_ascii=False, indent=2), encoding="utf-8")
    os.replace(temporary, path)


def models_executable() -> str | None:
    return os.environ.get("YOU_DUB_MODELS_EXECUTABLE") or shutil.which("you")


def inspect_model(model_name: str, server: str) -> dict | None:
    """Read public catalog facts only. Any failure returns None, never raises."""
    executable = models_executable()
    if not executable:
        return None
    arguments = [executable, "--json"]
    if server:
        arguments += ["--server", server]
    arguments += ["models", "inspect", model_name]
    try:
        result = subprocess.run(arguments, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                                timeout=INSPECT_TIMEOUT_SECONDS, check=False)
        if result.returncode:
            return None
        detail = json.loads(result.stdout.decode("utf-8-sig"))
    except (OSError, ValueError, subprocess.SubprocessError):
        return None
    return detail if isinstance(detail, dict) else None


def operator_config_path() -> Path:
    return Path(os.path.expanduser("~")).joinpath(*OPERATOR_CONFIG_PARTS)


def _text(mapping, name: str) -> str:
    value = mapping.get(name) if isinstance(mapping, dict) else None
    return value.strip() if isinstance(value, str) else ""


def _configured_profile(model_name: str, identity: str) -> dict | None:
    """Bind the operator's configured model overlay by digest, never raw values."""
    path = operator_config_path()
    if not path.is_file():
        return {"configured": False, "model": "", "sha256": _fingerprint({"model": None, "overlay": None})}
    try:
        document = json.loads(path.read_text(encoding="utf-8-sig"))
    except (OSError, UnicodeDecodeError, json.JSONDecodeError):
        return None
    models = document.get("models") if isinstance(document, dict) else None
    matched, overlay = "", None
    if isinstance(models, dict):
        for name in (model_name, identity):
            if name and isinstance(models.get(name), dict):
                matched, overlay = name, models[name]
                break
    # A declared backend is a mutable runtime selection: neither the managed
    # revision nor its asset bytes describe what that backend actually loaded.
    if isinstance(overlay, dict) and _text(overlay, "backend"):
        return None
    return {"configured": overlay is not None, "model": matched,
            "sha256": _fingerprint({"model": matched or None, "overlay": overlay})}


def _cache_root(cache_path) -> Path | None:
    """The installed asset directory, or None when the catalog named none.

    Absent, empty, and unusable values are ordinary: they only disable reuse.
    An empty string in particular must never become the current directory.
    """
    if not isinstance(cache_path, (str, os.PathLike)) or not str(cache_path).strip():
        return None
    try:
        return Path(cache_path).expanduser()
    except (OSError, ValueError):
        return None


def _cache_assets(cache_path) -> dict:
    """Digest the installed bytes so a silent asset replacement cannot be reused.

    A managed model may install its weights as links instead of plain files. The
    real bytes behind a link cannot be read this way, so this never returns a
    hash of metadata alone: unprovable weights yield no assets, which disables
    reuse rather than asserting a provenance that was never proven.
    """
    root = _cache_root(cache_path)
    if root is None or not root.is_dir():
        return {}
    assets = {}
    for path in sorted(root.rglob("*")):
        if path.is_symlink():
            return {}
        if not path.is_file():
            continue
        try:
            assets[path.relative_to(root).as_posix()] = file_sha256(path)
        except (OSError, ValueError):
            return {}
    return assets


def generation_policy(language: str) -> dict:
    """Reference voice, language, current speed cap, and nonverbal policy."""
    native = dub_contract.LANGUAGES[dub_contract.target_language(language).split("-")[0]][0]
    policy = {"identifier": POLICY_IDENTIFIER, "reference_voice": True, "language": language,
              "tts_language": native, "maximum_speed": MAXIMUM_SPEECH_SPEED,
              "nonverbal": NONVERBAL_POLICY}
    return {**policy, "sha256": _fingerprint(policy)}


def tts_provenance(value) -> dict | None:
    """Capture one stable synthesis identity, or None when it cannot be proven.

    A remote Models server runs the model on another host, so this process can
    never bind the actual weights it used. The requested catalog name alone is
    never a sufficient identity, and nothing here fabricates one.
    """
    model_name = value["models"]["tts"]
    if value.get("tts_server") or "":
        return None
    detail = inspect_model(model_name, "")
    if detail is None:
        return None
    runtime = detail.get("managedRuntime")
    diagnostics = runtime.get("diagnostics") if isinstance(runtime, dict) else None
    if not isinstance(diagnostics, dict):
        diagnostics = detail.get("diagnostics")
    identity, revision = _text(runtime, "identity"), _text(runtime, "revision")
    source_id, source_kind = _text(diagnostics, "sourceId"), _text(diagnostics, "sourceKind")
    profile = _configured_profile(model_name, identity)
    if not all((identity, revision, source_id, source_kind)) or profile is None:
        return None
    assets = _cache_assets(runtime.get("cachePath") if isinstance(runtime, dict) else None)
    if not assets:
        return None
    provenance = {
        "requested_model": model_name, "requested_language": value["language"], "server": "",
        "managed_identity": identity, "managed_revision": revision,
        "source_id": source_id, "source_kind": source_kind,
        "cache_asset_sha256": assets, "profile": profile,
        "generation_policy": generation_policy(value["language"]),
    }
    provenance["signature"] = _fingerprint(provenance)
    return provenance


def cue_presentation(cue: dict) -> dict:
    """Presentation derived by this run's fit: origin, timing, speed, kind."""
    return {key: cue.get(key) for key in CUE_PRESENTATION_KEYS}


def cue_input(cue: dict) -> dict:
    """The exact accepted input this cue was fitted from."""
    return {key: cue.get(key) for key in CUE_INPUT_KEYS}


def _bound_cue(accepted: dict, recorded: dict, checkpoint: dict, kind) -> bool:
    """Prove the fitted audio belongs to the cue now accepted for this record.

    The recorded cue and the accepted cue are both read from the same editable
    translations document, so an edited target text looks unchanged to a
    comparison between them. The stamp taken when the cue actually fitted is
    the independent binding: its fingerprint must still describe its own
    fields, its input must be the accepted input, the classification it applied
    must be the one the accepted text still calls for, and its presentation
    must be the presentation recorded beside it.
    """
    cue = checkpoint.get("cue")
    if not isinstance(cue, dict):
        return False
    stamp = {**cue_input(cue), **cue_presentation(cue)}
    if cue.get("cue_sha256") != _fingerprint(stamp):
        return False
    if any(stamp[key] != accepted.get(key) for key in CUE_INPUT_KEYS):
        return False
    if stamp["audio_origin"] not in (REFERENCE_CONDITIONED, SOURCE_NONVERBAL) \
            or bool(kind) != (stamp["audio_origin"] == SOURCE_NONVERBAL) \
            or stamp["nonverbal_kind"] != (kind or None):
        return False
    return cue_presentation(stamp) == cue_presentation(recorded)


def cue_checkpoint(working: dict, provenance: dict, video_sha256: str) -> dict:
    fitted = Path(working["fitted_audio"])
    cue = {**cue_input(working), **cue_presentation(working)}
    return {"version": CHECKPOINT_VERSION, "video_sha256": video_sha256,
            "reference_sha256": file_sha256(working["reference_audio"]),
            "speech_sha256": file_sha256(working["speech_audio"]),
            "fitted_sha256": file_sha256(fitted), "fitted_bytes": fitted.stat().st_size,
            "cue": {**cue, "cue_sha256": _fingerprint(cue)}, "provenance": provenance}


def playback_limit(accepted, index: int, duration_ms) -> int:
    """The trailing silent gap bounds one cue; a reuse must stay inside it."""
    following = accepted[index + 1]["start"] if index + 1 < len(accepted) else None
    return duration_ms if following is None else following


def _scratch(root: Path, suffix: str) -> Path:
    handle, name = tempfile.mkstemp(dir=root, suffix=suffix)
    os.close(handle)
    return Path(name)


def _matches_recomputation(origin, root, paths, accepted, limit, frames, fitted_sha256) -> bool:
    """Rebuild the fitted PCM on CPU and demand byte-identical evidence."""
    probe = {key: accepted[key] for key in ("id", "start", "end")}
    scratch = _scratch(root, ".pcm")
    try:
        if origin == SOURCE_NONVERBAL:
            dub_media.preserve_nonverbal(paths["reference_audio"], scratch, probe)
        else:
            dub_media.fit_speech(paths["speech_audio"], scratch, probe, limit)
        if scratch.stat().st_size != frames * BYTES_PER_SAMPLE:
            return False
        return file_sha256(scratch) == fitted_sha256
    except (ValueError, OSError):
        return False
    finally:
        scratch.unlink(missing_ok=True)


def _regular_artifact(root, recorded: str) -> Path | None:
    """A checkpoint may only name a regular file inside its own artifact root."""
    if not isinstance(recorded, str) or not recorded.strip():
        return None
    try:
        path = Path(recorded).resolve(strict=True)
    except OSError:
        return None
    return path if path.is_file() and path.is_relative_to(root) else None


def _timing(origin, recorded, limit):
    """Exact playback end and fitted frame count, or None when they cannot hold."""
    start, end = recorded.get("start"), recorded.get("end")
    speech_end, speed = recorded.get("speech_end"), recorded.get("speech_speed")
    if any(type(value) is not int for value in (start, end, speech_end)):
        return None
    if isinstance(speed, bool) or not isinstance(speed, (int, float)):
        return None
    if origin == SOURCE_NONVERBAL:
        if speech_end != end or speed != 1.0:
            return None
    elif not 1.0 <= speed <= MAXIMUM_SPEECH_SPEED or not end <= speech_end <= limit:
        return None
    return (speech_end - start) * SAMPLES_PER_MILLISECOND


def verified_cue(root, accepted, recorded, provenance, video_sha256, limit, preserve_names):
    """Accept one completed cue only while every bound input still matches."""
    if not isinstance(recorded, dict) or not isinstance(accepted, dict):
        return None
    checkpoint = recorded.get(CHECKPOINT_KEY)
    if not isinstance(checkpoint, dict) or checkpoint.get("version") != CHECKPOINT_VERSION:
        return None
    # Raw source identity plus the currently accepted target text and language.
    if any(recorded.get(key) != accepted.get(key) for key in CUE_INPUT_KEYS):
        return None
    if checkpoint.get("video_sha256") != video_sha256 or checkpoint.get("provenance") != provenance:
        return None
    origin = recorded.get("audio_origin")
    kind = dub_contract.nonverbal_kind(accepted["source_text"], accepted["text"], preserve_names)
    # The completion stamp binds the accepted input, the classification that
    # produced this audio, and the presentation that was really applied. A record
    # compared only with itself cannot show any of those.
    if not _bound_cue(accepted, recorded, checkpoint, kind):
        return None
    frames = _timing(origin, recorded, limit)
    if frames is None:
        return None
    paths = {key: _regular_artifact(root, recorded.get(key))
             for key in ("reference_audio", "speech_audio", "fitted_audio")}
    if any(path is None for path in paths.values()):
        return None
    fitted_sha256 = file_sha256(paths["fitted_audio"])
    if not (file_sha256(paths["reference_audio"]) == recorded.get("reference_sha256")
            == checkpoint.get("reference_sha256")):
        return None
    if file_sha256(paths["speech_audio"]) != checkpoint.get("speech_sha256"):
        return None
    if fitted_sha256 != checkpoint.get("fitted_sha256") or paths["fitted_audio"].stat().st_size \
            != frames * BYTES_PER_SAMPLE or checkpoint.get("fitted_bytes") != frames * BYTES_PER_SAMPLE:
        return None
    if origin == SOURCE_NONVERBAL and paths["speech_audio"] != paths["reference_audio"]:
        return None
    if not _matches_recomputation(origin, root, paths, accepted, limit, frames, fitted_sha256):
        return None
    return recorded


def _recorded_checkpoint(recorded, index: int) -> bool:
    return index < len(recorded) and isinstance(recorded[index], dict) \
        and isinstance(recorded[index].get(CHECKPOINT_KEY), dict)


class Progress:
    """One synthesize pass: verified reuse plus durable per-cue writes."""

    def __init__(self, root, value, accepted, recorded):
        self.path = Path(value["translations"])
        self.root = Path(root).resolve(strict=True)
        self.value = value
        self.accepted = accepted
        self.recorded = recorded if isinstance(recorded, list) else []
        self._captured, self._provenance, self._video = False, None, None
        self.completed = [self._verified(index) for index in range(len(accepted))]

    def _identity(self):
        # Captured at most once per stage, and only when a cue completes or a
        # recorded checkpoint has to be judged against it.
        if not self._captured:
            self._captured, self._provenance = True, tts_provenance(self.value)
        return self._provenance

    def _video_sha256(self):
        if self._video is None:
            self._video = file_sha256(self.value["video"])
        return self._video

    def _verified(self, index):
        if not _recorded_checkpoint(self.recorded, index):
            return None
        provenance = self._identity()
        if provenance is None:
            return None
        limit = playback_limit(self.accepted, index, self.value["duration_ms"])
        return verified_cue(self.root, self.accepted[index], self.recorded[index], provenance,
                            self._video_sha256(), limit, self.value.get("preserve_names", []))

    def complete(self, working: dict) -> dict:
        """Record durable progress, or leave the cue unclaimed when unprovable."""
        provenance = self._identity()
        if provenance is None:
            return working
        return {**working, CHECKPOINT_KEY: cue_checkpoint(working, provenance, self._video_sha256())}

    def save(self) -> None:
        """Persist progress without disturbing the language or the segment set.

        A cue that is not verified-complete keeps its plain accepted translation,
        so the artifact always stays a valid translations document.
        """
        segments = [self.completed[index] if self.completed[index] is not None
                    else dict(self.accepted[index]) for index in range(len(self.accepted))]
        write_json(self.path, {"language": self.value["language"], "segments": segments})
