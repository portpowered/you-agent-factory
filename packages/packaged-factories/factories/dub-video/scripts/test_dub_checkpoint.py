"""Only a still-valid completed cue survives a later synthesis failure."""

import json
import subprocess
import tempfile
import unittest
from contextlib import ExitStack
from pathlib import Path
from unittest.mock import patch

import dub_checkpoint
import dub_media
import dub_video
from dub_media import CommandFailed, SpeechDoesNotFit

LANGUAGE = "zh-CN"
SOURCE = [
    {"id": 0, "start": 0, "end": 400, "text": "Can you help me?"},
    {"id": 1, "start": 400, "end": 900, "text": "Thank you very much."},
    {"id": 2, "start": 900, "end": 1500, "text": "Mm."},
]
TARGET = ["你能帮我吗？", "非常感谢。", "嗯嗯"]
LEXICAL = TARGET[:2]
EDITED = "你可以帮我吗？"
# Distinguishes an omitted cachePath from a catalog that named one.
ABSENT = object()
REFERENCE_CONDITIONED = dub_checkpoint.REFERENCE_CONDITIONED
SOURCE_NONVERBAL = dub_checkpoint.SOURCE_NONVERBAL
SAMPLE_BYTES = 2
SAMPLES_PER_MS = 24
# Distinguishes "inspect returns the modeled catalog fact" from "no fact at all".
INSPECTED = object()


def exhaustion():
    return CommandFailed("you", 1, json.dumps({"code": "MODEL_BACKEND_FAILURE",
                                               "message": "TTS generation limit reached without EOS"}))


def pcm(segment):
    """PCM16 bytes the rendered timeline demands for one fitted cue."""
    return (segment["speech_end"] - segment["start"]) * SAMPLES_PER_MS * SAMPLE_BYTES


def fit_speech(source, destination, segment, playback_limit):
    segment["speech_end"] = segment["end"]
    destination.write_bytes(b"\x01\x00" * samples(segment))
    return 1.0


def preserve_nonverbal(source, destination, segment):
    segment["speech_end"] = segment["end"]
    destination.write_bytes(b"\x02\x00" * samples(segment))


def samples(segment):
    """PCM16 samples one fitted cue must contain."""
    return (segment["speech_end"] - segment["start"]) * SAMPLES_PER_MS


def speech_edge(observation, audio=b"generated speech"):
    """The TTS dispatch edge: record every requested cue, then serve audio."""
    def model(name, operation, inputs, outputs, parameters=None, server=""):
        observation(inputs[0].removeprefix("text="))
        Path(outputs[0].removeprefix("audio=")).write_bytes(audio)
    return model


def failing_on(text):
    def observe(candidate):
        if candidate == text:
            raise exhaustion()
    return observe


class DubCheckpointTests(unittest.TestCase):
    def workspace(self, cues=3, tts_server=""):
        """A complete translated artifact whose media edges stay injectable."""
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        root = Path(directory.name)
        artifact = root / "artifact"
        artifact.mkdir()
        cache = root / "cache"
        cache.mkdir()
        (cache / "weights.bin").write_bytes(b"managed tts weights")
        config = root / "config.json"
        config.write_text(json.dumps({"models": {"tts": {"source": "file:///models/qwen3-tts"}}}),
                          encoding="utf-8")
        video = artifact / "source.mp4"
        video.write_bytes(b"original video")
        segments = [dict(item) for item in SOURCE[:cues]]
        translations = artifact / "translated-asr.json"
        dub_video.save_json(translations, {"language": LANGUAGE, "segments": [
            {"id": item["id"], "text": TARGET[index]} for index, item in enumerate(segments)]})
        manifest = artifact / "manifest.json"
        dub_video.save_json(manifest, {"version": 1, "root": str(artifact), "video": str(video),
            "output": str(root / "dubbed.mp4"), "duration_ms": segments[-1]["end"], "segments": segments,
            "language": LANGUAGE, "stage": "translated", "models": {"tts": "tts", "llm": "llm"},
            "tts_server": tts_server, "translations": str(translations),
            "asr": str(artifact / "source-asr.json"), "transcript": str(artifact / "source.txt"),
            "source_ass": ""})
        return {"root": root, "artifact": artifact, "manifest": manifest,
                "translations": translations, "config": config, "cache": cache}

    def inspect(self, workspace, identity="tts", revision="rev-1", source="file:///models/qwen3-tts",
                source_kind="UPSTREAM_REPOSITORY"):
        """Public `you --json models inspect <tts>` facts, injected at that edge."""
        return {"name": "tts", "managedRuntime": {"identity": identity, "revision": revision,
            "cachePath": str(workspace["cache"]),
            "diagnostics": {"sourceId": source, "sourceKind": source_kind}}}

    def record(self, workspace):
        return dub_video.read_json(workspace["translations"])["segments"]

    def save(self, workspace, segments):
        dub_video.save_json(workspace["translations"], {"language": LANGUAGE, "segments": segments})

    def retry(self, workspace):
        """A retried Factory run resumes from the translated work state."""
        manifest = dub_video.read_json(workspace["manifest"])
        manifest["stage"] = "translated"
        dub_video.save_json(workspace["manifest"], manifest)

    def stage(self, workspace, observation, detail=INSPECTED, fit=None, preserved=None):
        """Run one real synthesize stage over fully injected media edges."""
        def reference(video, segment, destination):
            destination.write_bytes(f"reference::{segment['id']}".encode())
        # A retried Factory run re-enters this step with the same manifest.
        self.retry(workspace)
        with ExitStack() as stack:
            enter = stack.enter_context
            enter(patch.object(dub_video, "compare_translation", return_value="Source comparison"))
            enter(patch.object(dub_video, "reference", side_effect=reference))
            enter(patch.object(dub_video, "model", side_effect=speech_edge(observation)))
            enter(patch.object(dub_video, "fit_speech", side_effect=fit or fit_speech))
            enter(patch.object(dub_video, "preserve_nonverbal",
                               side_effect=self.nonverbal_edge(preserved)))
            # Verification recomputes the fit on CPU through the same media edges.
            enter(patch.object(dub_media, "fit_speech", side_effect=fit_speech))
            enter(patch.object(dub_media, "preserve_nonverbal", side_effect=preserve_nonverbal))
            enter(patch.object(dub_checkpoint, "inspect_model",
                               return_value=self.inspect(workspace) if detail is INSPECTED else detail))
            enter(patch.object(dub_checkpoint, "operator_config_path", return_value=workspace["config"]))
            dub_video.synthesize(str(workspace["manifest"]))

    def nonverbal_edge(self, observed):
        """A nonverbal cue regenerates through this edge, never through TTS."""
        def preserve(source, destination, segment):
            if observed is not None:
                observed.append(segment["id"])
            preserve_nonverbal(source, destination, segment)
        return preserve

    def synthesize(self, workspace, detail=INSPECTED, observation=None, fit=None, preserved=None):
        requested = observation if observation is not None else []
        self.stage(workspace, observation or requested.append, detail=detail, fit=fit,
                   preserved=preserved)
        return requested

    def assert_regenerates(self, workspace, mutate, detail=INSPECTED, policy=None, expected=None,
                           resealed=False):
        """Change one bound fact after a clean run and prove affected cues rebuild."""
        self.synthesize(workspace)
        evidence = self.record(workspace)
        mutate(workspace)
        identifier = policy or dub_checkpoint.POLICY_IDENTIFIER
        with patch.object(dub_checkpoint, "POLICY_IDENTIFIER", identifier):
            requested = self.synthesize(workspace, detail=detail)
        self.assertEqual(requested, expected if expected is not None else LEXICAL,
                         "an unchanged verified cue must be skipped")
        if not resealed:
            # A changed text or identity must also change the record itself. For
            # media-only changes the deterministic edge restores identical valid
            # bytes, so the regeneration call above is the whole proof.
            self.assertNotEqual(self.record(workspace)[0], evidence[0])
        self.assertEqual(dub_video.read_json(workspace["manifest"])["stage"], "synthesized")

    def test_completed_cue_is_persisted_before_a_later_cue_fails(self):
        workspace = self.workspace()
        with self.assertRaises(CommandFailed):
            self.stage(workspace, failing_on(TARGET[1]))
        persisted = self.record(workspace)
        self.assertEqual([item["id"] for item in persisted], [0, 1, 2])
        checkpoint = persisted[0]["checkpoint"]
        self.assertEqual(checkpoint["version"], dub_checkpoint.CHECKPOINT_VERSION)
        self.assertEqual(persisted[0]["audio_origin"], REFERENCE_CONDITIONED)
        self.assertEqual(checkpoint["fitted_bytes"], pcm(persisted[0]))
        self.assertEqual(checkpoint["fitted_sha256"], dub_video.digest(Path(persisted[0]["fitted_audio"])))
        self.assertEqual(checkpoint["video_sha256"], dub_video.digest(workspace["artifact"] / "source.mp4"))
        # A failed or unreached cue stays a plain accepted translation.
        self.assertEqual(persisted[1], {"id": 1, "start": 400, "end": 900,
                                       "source_text": SOURCE[1]["text"], "text": TARGET[1]})
        self.assertNotIn("checkpoint", persisted[2])
        self.assertEqual(dub_video.read_json(workspace["manifest"])["stage"], "translated")

    def test_retry_skips_the_verified_cue_and_regenerates_only_the_failed_one(self):
        workspace = self.workspace()
        with self.assertRaises(CommandFailed):
            self.stage(workspace, failing_on(TARGET[1]))
        evidence = self.record(workspace)[0]
        self.assertEqual(self.synthesize(workspace), [TARGET[1]])
        persisted = self.record(workspace)
        self.assertEqual(persisted[0], evidence)
        self.assertEqual([item["audio_origin"] for item in persisted],
                         [REFERENCE_CONDITIONED, REFERENCE_CONDITIONED, SOURCE_NONVERBAL])
        self.assertEqual((persisted[2]["speech_speed"], persisted[2]["speech_end"]),
                         (1.0, SOURCE[2]["end"]))
        self.assertEqual(dub_video.read_json(workspace["manifest"])["stage"], "synthesized")

    def test_nonverbal_cue_reuses_only_inside_the_exact_source_interval(self):
        workspace = self.workspace()
        self.assertEqual(self.synthesize(workspace), LEXICAL)
        persisted = self.record(workspace)
        # A nonverbal cue regenerates through preserve_nonverbal, never through TTS.
        preserved = []
        self.assertEqual(self.synthesize(workspace, preserved=preserved), [])
        self.assertEqual(preserved, [])
        self.assertEqual(self.record(workspace), persisted)

        widened = self.record(workspace)
        widened[2]["speech_end"] = SOURCE[2]["end"] + 1
        self.save(workspace, widened)
        preserved = []
        self.assertEqual(self.synthesize(workspace, preserved=preserved), [])
        self.assertEqual(preserved, [2], "the widened bound must be rebuilt from source audio")
        self.assertEqual(self.record(workspace)[2]["speech_end"], SOURCE[2]["end"])
        self.assertEqual(self.record(workspace)[2]["fitted_audio"], persisted[2]["fitted_audio"])

    def test_changed_model_source_profile_or_policy_invalidates_completed_progress(self):
        def changed_cache(workspace):
            (workspace["cache"] / "weights.bin").write_bytes(b"replaced managed weights")

        def changed_profile(workspace):
            workspace["config"].write_text(json.dumps(
                {"models": {"tts": {"source": "file:///models/qwen3-tts-v2"}}}), encoding="utf-8")

        def declared_backend(workspace):
            workspace["config"].write_text(json.dumps(
                {"models": {"tts": {"source": "file:///models/qwen3-tts", "backend": "http://host"}}}),
                encoding="utf-8")

        nothing = lambda workspace: None
        changed = {"identity": {"identity": "other-tts"}, "revision": {"revision": "rev-2"},
                   "source": {"source": "file:///other"}, "source_kind": {"source_kind": "MANAGED_MIRROR"}}
        for name, mutate, detail, policy in (
                ("identity", nothing, "identity", None),
                ("revision", nothing, "revision", None),
                ("source", nothing, "source", None),
                ("source-kind", nothing, "source_kind", None),
                ("cache-bytes", changed_cache, INSPECTED, None),
                ("profile", changed_profile, INSPECTED, None),
                ("declared-backend", declared_backend, INSPECTED, None),
                ("generation-policy", nothing, INSPECTED, "dub-video/other/1")):
            with self.subTest(fact=name):
                workspace = self.workspace()
                arguments = {} if detail is INSPECTED else {"detail": self.inspect(workspace, **changed[detail])}
                # A stage-wide identity or policy change retires every recorded cue.
                self.assert_regenerates(workspace, mutate, policy=policy, **arguments)

    def test_changed_reference_speech_pcm_timing_or_text_invalidates_completed_progress(self):
        def rewrite(key, replacement):
            def mutate(workspace):
                Path(self.record(workspace)[0][key]).write_bytes(replacement)
            return mutate

        def reseal_timing(workspace):
            """Move the stored end, then re-seal every hash so only timing differs."""
            stored = self.record(workspace)
            segment = stored[0]
            segment["speech_end"] = SOURCE[0]["end"] - 40
            fitted = Path(segment["fitted_audio"])
            fitted.write_bytes(b"\x01\x00" * samples(segment))
            segment["checkpoint"]["fitted_sha256"] = dub_video.digest(fitted)
            segment["checkpoint"]["fitted_bytes"] = fitted.stat().st_size
            self.save(workspace, stored)

        def edit_text(workspace):
            """Editing the target text invalidates the audio fitted from it."""
            stored = self.record(workspace)
            stored[0]["text"] = EDITED
            self.save(workspace, stored)

        # A deterministic edge restores valid identical bytes on rebuild, so
        # only the actual regeneration call distinguishes the media cases. The
        # text case is a real defect: the regeneration speaks the edited text
        # and its record genuinely differs.
        for name, mutate, resealed, requested in (
                ("reference", rewrite("reference_audio", b"replaced reference bytes"), True, [TARGET[0]]),
                ("speech", rewrite("speech_audio", b"replaced speech bytes"), True, [TARGET[0]]),
                ("fitted-pcm", rewrite("fitted_audio", b"\x09\x00" * 400), True, [TARGET[0]]),
                ("timing", reseal_timing, True, [TARGET[0]]),
                ("text", edit_text, False, [EDITED])):
            with self.subTest(fact=name):
                # Only the cue whose own evidence changed is rebuilt.
                self.assert_regenerates(self.workspace(), mutate, expected=requested,
                                        resealed=resealed)

    def test_regenerated_cue_stamp_binds_the_accepted_text_it_was_fitted_from(self):
        workspace = self.workspace()
        self.synthesize(workspace)
        stored = self.record(workspace)
        stored[0]["text"] = EDITED
        self.save(workspace, stored)
        # The edited text is not the text this cue's audio speaks.
        self.assertEqual(self.synthesize(workspace), [EDITED])
        rebound = self.record(workspace)[0]
        self.assertEqual((rebound["text"], rebound["checkpoint"]["cue"]["text"]), (EDITED, EDITED))
        self.assertNotEqual(rebound["checkpoint"]["cue"]["cue_sha256"],
                            stored[0]["checkpoint"]["cue"]["cue_sha256"])
        # The fresh stamp binds the new text, so an untouched rerun reuses it.
        self.assertEqual(self.synthesize(workspace), [])
        self.assertEqual(self.record(workspace)[0], rebound)

    def test_incomplete_or_foreign_metadata_is_never_reused(self):
        def drop(*paths):
            """Delete a documented metadata path from every recorded cue."""
            def mutate(workspace):
                stored = self.record(workspace)
                for segment in stored:
                    for path in paths:
                        keys = path.split(".")
                        cursor = segment
                        for key in keys[:-1]:
                            cursor = cursor.get(key)
                            if not isinstance(cursor, dict):
                                break
                        if isinstance(cursor, dict):
                            cursor.pop(keys[-1], None)
                self.save(workspace, stored)
            return mutate

        def wrong_version(workspace):
            stored = self.record(workspace)
            for segment in stored:
                segment["checkpoint"]["version"] = 99
            self.save(workspace, stored)

        def no_files(workspace):
            """Metadata that survived its media is not trusted either."""
            stored = self.record(workspace)
            for segment in stored:
                Path(segment["fitted_audio"]).unlink()
            self.save(workspace, stored)

        for name, mutate in {
                "no-checkpoint": drop("checkpoint"),
                "missing-version": drop("checkpoint.version"),
                "missing-provenance": drop("checkpoint.provenance"),
                "missing-fitted-hash": drop("checkpoint.fitted_sha256"),
                "missing-cue-stamp": drop("checkpoint.cue"),
                "wrong-version": wrong_version,
                "deleted-media": no_files,
        }.items():
            with self.subTest(shape=name):
                workspace = self.workspace()
                self.synthesize(workspace)
                mutate(workspace)
                preserved = []
                # Every incomplete or foreign shape retires every lexical cue.
                self.assertEqual(self.synthesize(workspace, preserved=preserved), LEXICAL)
                self.assertEqual(preserved, [2])
                self.assertEqual(dub_video.read_json(workspace["manifest"])["stage"], "synthesized")

    def test_an_unusable_inspected_cache_path_only_disables_reuse(self):
        """Public metadata never becomes a prerequisite for synthesis itself."""
        for name in ("absent", "missing", "none", "empty", "blank", "invalid",
                     "wrong-type", "not-a-directory"):
            with self.subTest(cache=name):
                workspace = self.workspace()
                inspected = self.inspect(workspace)
                shape = {
                    "absent": ABSENT, "none": None, "empty": "", "blank": "   ",
                    "invalid": "\x00", "wrong-type": 7,
                    "missing": str(workspace["root"] / "never-installed"),
                    "not-a-directory": str(workspace["cache"] / "weights.bin"),
                }[name]
                if shape is ABSENT:
                    del inspected["managedRuntime"]["cachePath"]
                else:
                    inspected["managedRuntime"]["cachePath"] = shape
                self.assertEqual(dub_checkpoint._cache_assets(inspected["managedRuntime"].get("cachePath")), {})
                self.assertEqual(self.synthesize(workspace, detail=inspected), LEXICAL,
                                 "synthesis still happens normally")
                persisted = self.record(workspace)
                self.assertEqual([item["audio_origin"] for item in persisted],
                                 [REFERENCE_CONDITIONED, REFERENCE_CONDITIONED, SOURCE_NONVERBAL])
                self.assertTrue(all("checkpoint" not in item for item in persisted),
                                "unprovable weights never claim a cache hit")
                # Every fitted PCM still exists and still holds the silence and
                # speech the renderer needs.
                self.assertTrue(all(Path(item["fitted_audio"]).stat().st_size == pcm(item)
                                    for item in persisted))
                self.assertEqual(dub_video.read_json(workspace["manifest"])["stage"], "synthesized")

    def test_symlinked_model_assets_are_never_hashed_as_metadata_only(self):
        workspace = self.workspace()
        self.assertEqual(dub_checkpoint._cache_assets(str(workspace["cache"])),
                         {"weights.bin": dub_video.digest(workspace["cache"] / "weights.bin")})
        weights = workspace["cache"] / "weights.bin"
        resolved = workspace["root"] / "managed-weights.bin"
        resolved.write_bytes(weights.read_bytes())
        weights.unlink()
        try:
            weights.symlink_to(resolved)
        except (OSError, NotImplementedError) as error:
            self.skipTest(f"this platform cannot install model assets as links: {error}")
        # Linked weights hide the real bytes, so the identity stays unprovable
        # instead of being hashed from whatever metadata a link exposes.
        self.assertEqual(dub_checkpoint._cache_assets(str(workspace["cache"])), {})
        requested = self.synthesize(workspace, detail=self.inspect(workspace))
        self.assertEqual(requested, LEXICAL)
        self.assertTrue(all("checkpoint" not in item for item in self.record(workspace)))
        self.assertEqual(dub_video.read_json(workspace["manifest"])["stage"], "synthesized")

    def test_a_partly_unreadable_asset_cache_proves_nothing_and_disables_reuse(self):
        workspace = self.workspace()
        with patch.object(dub_checkpoint, "file_sha256", side_effect=OSError("asset is locked")):
            self.assertEqual(dub_checkpoint._cache_assets(str(workspace["cache"])), {},
                             "a partly readable cache is never a partial identity")
            requested = self.synthesize(workspace, detail=self.inspect(workspace))
            self.assertEqual(requested, LEXICAL)
            self.assertTrue(all("checkpoint" not in item for item in self.record(workspace)))
        self.assertEqual(dub_video.read_json(workspace["manifest"])["stage"], "synthesized")

    def test_bounded_fit_revision_keeps_accepted_text_and_evidence_across_reuse(self):
        workspace = self.workspace(cues=2)
        revised = "能帮我一下吗？"
        repairs, requested = [], []
        failing = {"cue": True}

        def inference(name, operation, inputs, outputs, parameters=None, server=""):
            if operation == "TTS":
                text = inputs[0].removeprefix("text=")
                requested.append(text)
                # The unreached cue fails the whole first stage only; the retried
                # Factory run must find the backend healthy again.
                if text == TARGET[1] and failing["cue"]:
                    raise exhaustion()
                Path(outputs[0].removeprefix("audio=")).write_bytes(b"generated speech")
                return
            prompt = Path(inputs[0].removeprefix("prompt=@"))
            content = prompt.read_text(encoding="utf-8")
            if prompt.name.startswith("fit-translation"):
                repairs.append(json.loads(content.split("\n")[-1])["segment_id"])
                reply = {"language": LANGUAGE, "segments": [{"id": 0, "text": revised}]}
            else:
                reply = {"valid": True, "issues": []}
            dub_video.save_json(Path(outputs[0].removeprefix("text=")), reply)

        def fit(source, destination, segment, playback_limit):
            if segment["text"] == TARGET[0]:
                raise SpeechDoesNotFit(segment["id"], 800, 400, 2.0)
            return fit_speech(source, destination, segment, playback_limit)

        def reference(video, segment, destination):
            destination.write_bytes(b"original reference")

        with ExitStack() as stack:
            enter = stack.enter_context
            enter(patch.object(dub_video, "compare_translation", return_value="Source comparison"))
            enter(patch.object(dub_video, "reference", side_effect=reference))
            enter(patch.object(dub_video, "fit_speech", side_effect=fit))
            enter(patch.object(dub_media, "fit_speech", side_effect=fit_speech))
            enter(patch.object(dub_checkpoint, "inspect_model", return_value=self.inspect(workspace)))
            enter(patch.object(dub_checkpoint, "operator_config_path", return_value=workspace["config"]))
            enter(patch.object(dub_video, "model", side_effect=inference))
            self.retry(workspace)
            with self.assertRaises(CommandFailed):
                dub_video.synthesize(str(workspace["manifest"]))
            # Nothing is left as an empty slot: cue 0 fitted and cue 1 still
            # holds the translation the next attempt will speak.
            interrupted = self.record(workspace)
            self.assertEqual([item["id"] for item in interrupted], [0, 1])
            self.assertTrue(all(isinstance(item, dict) and item.get("text") for item in interrupted))
            recovered = interrupted[0]
            self.assertEqual(recovered["text"], revised)
            self.assertEqual([item["status"] for item in recovered["fit_attempts"]],
                             ["rejected", "fitted"])
            self.assertEqual(recovered["checkpoint"]["cue"]["text"], revised)
            failing["cue"] = False
            requested.clear()
            self.retry(workspace)
            dub_video.synthesize(str(workspace["manifest"]))
        self.assertEqual(repairs, [0], "an accepted revision is never re-repaired")
        self.assertEqual(requested, [TARGET[1]], "a revised cue is reused, not resynthesized")
        reused = self.record(workspace)
        self.assertEqual(reused[0], recovered)
        self.assertEqual(reused[0]["text"], revised)
        self.assertEqual([item["status"] for item in reused[0]["fit_attempts"]], ["rejected", "fitted"])
        self.assertEqual(reused[1]["text"], TARGET[1])
        self.assertEqual([item["status"] for item in reused[1]["tts_attempts"]], ["succeeded"])
        self.assertEqual(dub_video.read_json(workspace["manifest"])["stage"], "synthesized")

    def test_unprovable_model_identity_still_synthesizes_without_claiming_progress(self):
        for name, workspace in (("no-inspect-facts", self.workspace()),
                                ("remote-server", self.workspace(tts_server="http://127.0.0.1:9999"))):
            with self.subTest(identity=name):
                requested = self.synthesize(workspace, detail=None)
                self.assertEqual(requested, LEXICAL, "synthesis still happens normally")
                persisted = self.record(workspace)
                self.assertEqual([item["audio_origin"] for item in persisted],
                                 [REFERENCE_CONDITIONED, REFERENCE_CONDITIONED, SOURCE_NONVERBAL])
                self.assertTrue(all("checkpoint" not in item for item in persisted),
                                "an unprovable identity never claims a cache hit")
                self.assertEqual(dub_video.read_json(workspace["manifest"])["stage"], "synthesized")

    def test_inspection_failure_is_not_a_prerequisite_for_inference(self):
        workspace = self.workspace()
        requested = []
        # The public inspect edge itself fails; inference must still be reachable.
        self.synthesize(workspace, detail=None, observation=requested.append)
        self.assertEqual(requested, LEXICAL)
        self.assertEqual(dub_video.read_json(workspace["manifest"])["stage"], "synthesized")

    def test_a_crashed_inspect_process_cannot_block_speech_generation(self):
        workspace = self.workspace()
        requested = []

        def crashed(*arguments, **keywords):
            raise subprocess.TimeoutExpired(cmd="you", timeout=1.0)

        with patch.object(dub_checkpoint.subprocess, "run", side_effect=crashed), \
             patch.object(dub_checkpoint, "models_executable", return_value="/usr/bin/you"), \
             patch.object(dub_checkpoint, "operator_config_path", return_value=workspace["config"]):
            self.assertIsNone(dub_checkpoint.tts_provenance(
                {"models": {"tts": "tts"}, "language": LANGUAGE, "tts_server": ""}))
            self.synthesize(workspace, detail=None, observation=requested.append)
        self.assertEqual(requested, LEXICAL)
        self.assertEqual(dub_video.read_json(workspace["manifest"])["stage"], "synthesized")
        self.assertTrue(all("checkpoint" not in item for item in self.record(workspace)))

    def test_capture_happens_once_per_stage_for_the_whole_run(self):
        workspace = self.workspace()
        inspected = []

        def inspect(model_name, server):
            inspected.append((model_name, server))
            return self.inspect(workspace)

        with ExitStack() as stack:
            enter = stack.enter_context
            enter(patch.object(dub_checkpoint, "inspect_model", side_effect=inspect))
            enter(patch.object(dub_checkpoint, "operator_config_path", return_value=workspace["config"]))
            enter(patch.object(dub_video, "reference",
                               side_effect=lambda v, s, d: d.write_bytes(b"reference")))
            enter(patch.object(dub_video, "model", side_effect=speech_edge(lambda text: None)))
            enter(patch.object(dub_video, "fit_speech", side_effect=fit_speech))
            enter(patch.object(dub_video, "preserve_nonverbal", side_effect=preserve_nonverbal))
            self.retry(workspace)
            dub_video.synthesize(str(workspace["manifest"]))
            first = len(inspected)
            self.retry(workspace)
            dub_video.synthesize(str(workspace["manifest"]))
        self.assertEqual(inspected[0], ("tts", ""), "the requested model is inspected, not a hash")
        self.assertEqual(first, 1, "provenance is captured once per stage, not once per cue")
        self.assertEqual(len(inspected), 2, "one capture per stage")

    def test_render_still_rejects_a_translation_changed_after_reuse(self):
        workspace = self.workspace()
        self.synthesize(workspace)
        stored = self.record(workspace)
        stored[0]["source_text"] = "Can you help at all?"
        self.save(workspace, stored)
        with patch.object(dub_video, "timeline") as assemble:
            with self.assertRaisesRegex(ValueError, "changed after synthesis"):
                dub_video.render(str(workspace["manifest"]))
        assemble.assert_not_called()


if __name__ == "__main__":
    unittest.main()
