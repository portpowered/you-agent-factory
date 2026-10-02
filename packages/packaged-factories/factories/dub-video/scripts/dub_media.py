"""Seekable reference extraction, timeline assembly, and subtitle muxing."""

from __future__ import annotations

import json
import math
import subprocess
from pathlib import Path

from dub_contract import LANGUAGES, target_language


def command(argv: list[str]) -> None:
    result = subprocess.run(argv, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE)
    if result.returncode:
        detail = result.stderr.decode("utf-8", errors="replace").strip()
        raise ValueError(f"{argv[0]} failed ({result.returncode}): {detail[-4000:]}")


def probe(path: Path) -> dict:
    result = subprocess.run(
        ["ffprobe", "-v", "error", "-show_format", "-show_streams", "-of", "json", str(path)],
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False,
    )
    if result.returncode:
        raise ValueError(f"Cannot inspect {path}: {result.stderr.decode('utf-8', errors='replace')[-4000:]}")
    return json.loads(result.stdout)


def video_duration(path: Path) -> int:
    metadata = probe(path)
    streams = metadata.get("streams", [])
    if not any(stream.get("codec_type") == "video" for stream in streams):
        raise ValueError("Input must contain a video stream")
    if not any(stream.get("codec_type") == "audio" for stream in streams):
        raise ValueError("Input video has no audio stream to transcribe or use as a voice reference")
    duration = float(metadata.get("format", {}).get("duration", "nan"))
    if not math.isfinite(duration) or duration <= 0:
        raise ValueError("Input video has no usable duration")
    return math.ceil(duration * 1000)


def reference(video: Path, segment: dict, destination: Path) -> None:
    command([
        "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y",
        "-ss", f"{segment['start'] / 1000:.3f}", "-i", str(video),
        "-t", f"{(segment['end'] - segment['start']) / 1000:.3f}",
        "-map", "0:a:0", "-vn", "-ac", "1", "-ar", "24000",
        "-af", f"aresample=24000:async=1:first_pts=0:min_hard_comp=0.001,apad,atrim=end_sample={(segment['end'] - segment['start']) * 24},asetpts=N/SR/TB",
        "-c:a", "pcm_s16le", "-rf64", "auto", str(destination),
    ])
    duration = float(probe(destination).get("format", {}).get("duration", "nan"))
    if not math.isfinite(duration) or duration <= 0:
        raise ValueError(f"Source segment {segment['id']} has no reference audio")


def fit_speech(source: Path, destination: Path, segment: dict) -> float:
    duration = float(probe(source).get("format", {}).get("duration", "nan"))
    target = (segment["end"] - segment["start"]) / 1000
    if not math.isfinite(duration) or duration <= 0:
        raise ValueError(f"TTS returned empty audio for segment {segment['id']}")
    speed = max(1.0, duration / target)
    if speed > 2.0:
        raise ValueError(
            f"Segment {segment['id']} needs {speed:.2f}x speech speed to fit; "
            "shorten its translation before rendering to avoid unintelligible dubbing"
        )
    command([
        "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-i", str(source),
        "-af", f"atempo={speed:.8f},apad,atrim=duration={target:.3f}",
        "-ac", "1", "-ar", "24000", "-c:a", "pcm_s16le", "-f", "s16le", str(destination),
    ])
    return speed


def silence(output, frames: int) -> None:
    while frames > 0:
        count = min(frames, 24000)
        output.write(b"\0\0" * count)
        frames -= count


def timeline(segments: list[dict], duration_ms: int, destination: Path) -> None:
    # Stream PCM, including gaps, rather than retaining the complete video audio.
    # Raw PCM has no RIFF 4 GiB length field.
    with destination.open("wb") as output:
        current = 0
        for segment in segments:
            start = segment["start"] * 24
            end = segment["end"] * 24
            if start < current:
                raise ValueError("Dubbing segments overlap")
            silence(output, start - current)
            current = start
            fitted = Path(segment["fitted_audio"])
            if fitted.stat().st_size != (end - start) * 2:
                raise ValueError(f"Fitted speech for segment {segment.get('id')} has the wrong sample count")
            with fitted.open("rb") as source:
                while current < end:
                    data = source.read(min(24000, end - current) * 2)
                    if not data:
                        raise ValueError("Fitted speech was truncated while reading")
                    if len(data) % 2:
                        raise ValueError("Fitted speech has an incomplete PCM16 sample")
                    output.write(data)
                    current += len(data) // 2
            silence(output, end - current)
            current = end
        silence(output, duration_ms * 24 - current)


def mux(video: Path, audio: Path, subtitles: Path, destination: Path, language: str) -> None:
    # MP4 uses ISO 639-2 codes, not customer-facing BCP47 language tags.
    language = target_language(language)
    container_language = language if destination.suffix.lower() == ".mkv" else LANGUAGES[language.split("-")[0]][1]
    args = ["ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-i", str(video),
            "-f", "s16le", "-ar", "24000", "-ac", "1", "-i", str(audio), "-i", str(subtitles),
            "-map", "0:v:0", "-map", "1:a:0", "-map", "2:0",
            "-c:v", "copy", "-c:a", "aac", "-b:a", "192k"]
    if destination.suffix.lower() == ".mkv":
        args += ["-c:s", "ass"]
    else:
        # MP4 stores text subtitles as mov_text; keep the original ASS sidecar.
        args += ["-c:s", "mov_text", "-movflags", "+faststart"]
    args += ["-metadata:s:a:0", f"language={container_language}", "-metadata:s:s:0", f"language={container_language}",
             "-disposition:s:0", "default", str(destination)]
    command(args)
    streams = probe(destination).get("streams", [])
    if not all(any(stream.get("codec_type") == kind for stream in streams) for kind in ("video", "audio", "subtitle")):
        raise ValueError("Rendered output is missing its video, dubbed audio, or subtitle stream")
