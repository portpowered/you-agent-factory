"""Controlled proofs for source references, validation, and exact timelines."""

import io
import json
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

import dub_media
import dub_video


class DubPipelineTests(unittest.TestCase):
    def test_main_reconfigures_windows_pipes_for_unicode_paths_and_errors(self):
        for failure in (False, True):
            with self.subTest(failure=failure):
                output, errors = io.BytesIO(), io.BytesIO()
                stdout = io.TextIOWrapper(output, encoding="cp1252")
                stderr = io.TextIOWrapper(errors, encoding="cp1252")
                result = {"video": "C:/视频/中文.mp4", "language": "zh-CN"}
                behavior = {"side_effect": ValueError("不能读取中文视频")} if failure else {"return_value": result}
                with patch.object(dub_video.sys, "stdout", stdout), \
                     patch.object(dub_video.sys, "stderr", stderr), \
                     patch.object(dub_video.sys, "argv", ["dub_video.py", "render", "--manifest", "manifest.json"]), \
                     patch.object(dub_video, "render", **behavior):
                    status = dub_video.main()
                    stdout.flush()
                    stderr.flush()
                self.assertEqual(status, 1 if failure else 0)
                if failure:
                    self.assertIn("不能读取中文视频", errors.getvalue().decode("utf-8"))
                    self.assertEqual(output.getvalue(), b"")
                else:
                    self.assertEqual(json.loads(output.getvalue().decode("utf-8")), result)
                    self.assertEqual(errors.getvalue(), b"")
                stdout.detach()
                stderr.detach()

    def manifest(self, root, stage="translated"):
        segments = [{"id": 7, "start": 40, "end": 60, "text": "Can you help?"}]
        source = root / "source.mp4"
        source.write_bytes(b"source video")
        translations = root / "translated.json"
        dub_video.save_json(translations, {"language": "zh-CN", "segments": [
            {"id": 7, "start": 40, "end": 60, "source_text": "Can you help?", "text": "你能帮我吗？"}
        ]})
        path = root / "manifest.json"
        dub_video.save_json(path, {"version": 1, "root": str(root), "video": str(source),
            "duration_ms": 100, "segments": segments, "language": "zh-CN", "stage": stage,
            "models": {"tts": "qwen3-tts-base", "llm": "llm"}, "tts_server": "",
            "translations": str(translations), "output": str(root / "dubbed.mp4")})
        return path

    def test_each_tts_call_uses_original_audio_and_source_transcript(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            manifest = self.manifest(root)
            calls = []

            def extract(video, segment, destination):
                self.assertEqual(video.read_bytes(), b"source video")
                self.assertEqual((segment["id"], segment["start"], segment["end"]), (7, 40, 60))
                destination.write_bytes(b"original voice reference")

            def infer(name, operation, inputs, outputs, parameters, server):
                calls.append((name, operation, inputs, parameters))
                reference = Path(inputs[1].removeprefix("voice=@"))
                self.assertEqual(reference.read_bytes(), b"original voice reference")
                Path(outputs[0].removeprefix("audio=")).write_bytes(b"conditioned speech")

            def fit(source, destination, segment):
                self.assertEqual(source.read_bytes(), b"conditioned speech")
                destination.write_bytes(b"\x01\x00" * 480)
                return 1.0

            with patch.object(dub_video, "reference", side_effect=extract), \
                 patch.object(dub_video, "model", side_effect=infer), \
                 patch.object(dub_video, "fit_speech", side_effect=fit):
                dub_video.synthesize(str(manifest))
            self.assertEqual(len(calls), 1)
            name, operation, inputs, parameters = calls[0]
            self.assertEqual((name, operation), ("qwen3-tts-base", "TTS"))
            self.assertEqual(inputs[0], "text=你能帮我吗？")
            self.assertEqual(parameters, {"ref_text": "Can you help?", "language": "Chinese"})
            result = dub_video.read_json(manifest)
            self.assertEqual(result["stage"], "synthesized")
            item = dub_video.read_json(Path(result["translations"]))["segments"][0]
            self.assertEqual((item["id"], item["start"], item["end"]), (7, 40, 60))
            self.assertEqual(item["reference_sha256"], dub_video.digest(Path(item["reference_audio"])))

    def test_transcription_stores_trimmed_operator_glossary_in_manifest(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            video = root / "source video.mp4"
            video.write_bytes(b"source")
            args = SimpleNamespace(video=str(video), output=str(root / "dubbed.mp4"), language="zh-CN",
                subtitles="", asr_model="asr", llm_model="llm", tts_model="qwen3-tts-base", tts_server="",
                preserve_names="  Silver Clouds  \n\nAlice Smith\r\n ")

            def infer(name, operation, inputs, outputs):
                self.assertEqual((name, operation, inputs), ("asr", "ASR", [f"audio=@{video}"]))
                Path(outputs[0].removeprefix("transcript=")).write_text("Silver Clouds", encoding="utf-8")
                dub_video.save_json(Path(outputs[1].removeprefix("segments=")),
                                    [{"id": 7, "start": 0, "end": 100, "text": "Silver Clouds"}])

            with patch.object(dub_video, "video_duration", return_value=100), \
                 patch.object(dub_video, "model", side_effect=infer):
                manifest = dub_video.transcribe(args)
            self.assertEqual(dub_video.read_json(manifest)["preserve_names"], ["Silver Clouds", "Alice Smith"])

    def test_translated_glossary_name_is_rejected_before_audit_or_tts(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            manifest = self.manifest(root, stage="transcribed")
            value = dub_video.read_json(manifest)
            value["segments"][0]["text"] = "Please play silver clouds."
            value["preserve_names"] = ["Silver Clouds"]
            dub_video.save_json(manifest, value)

            def infer(name, operation, inputs, outputs):
                prompt = Path(inputs[0].removeprefix("prompt=@"))
                self.assertFalse(prompt.name.startswith("translation-audit"))
                self.assertEqual(operation, "OMNI")
                dub_video.save_json(Path(outputs[0].removeprefix("text=")),
                    {"language": "zh-CN", "segments": [{"id": 7, "text": "请播放银色的云。"}]})

            with patch.object(dub_video, "model", side_effect=infer) as infer:
                with self.assertRaisesRegex(ValueError, "three attempts.*preserve.*Silver Clouds"):
                    dub_video.translate(str(manifest))
                with self.assertRaisesRegex(ValueError, "requires validated"):
                    dub_video.synthesize(str(manifest))
            self.assertEqual(infer.call_count, 3)
            self.assertEqual(dub_video.read_json(manifest)["stage"], "transcribed")

    def test_glossary_is_revalidated_before_synthesis_and_render(self):
        for stage, operation in [("translated", dub_video.synthesize), ("synthesized", dub_video.render)]:
            with self.subTest(stage=stage), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                manifest = self.manifest(root, stage=stage)
                value = dub_video.read_json(manifest)
                value["segments"][0]["text"] = "Please play silver clouds."
                value["preserve_names"] = ["Silver Clouds"]
                dub_video.save_json(manifest, value)
                with patch.object(dub_video, "model") as infer, patch.object(dub_video, "timeline") as assemble:
                    with self.assertRaisesRegex(ValueError, "full name.*Silver Clouds"):
                        operation(str(manifest))
                infer.assert_not_called()
                assemble.assert_not_called()

    def test_invalid_translation_never_reaches_tts(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            manifest = self.manifest(root)
            data = dub_video.read_json(root / "translated.json")
            data["segments"][0]["id"] = 8
            dub_video.save_json(root / "translated.json", data)
            with patch.object(dub_video, "model") as infer:
                with self.assertRaisesRegex(ValueError, "IDs and order"):
                    dub_video.synthesize(str(manifest))
            infer.assert_not_called()
            self.assertEqual(dub_video.read_json(manifest)["stage"], "translated")

    def test_translation_failure_is_bounded_and_keeps_raw_response(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            manifest = self.manifest(root, stage="transcribed")

            def infer(name, operation, inputs, outputs):
                self.assertEqual([item.split('=', 1)[0] for item in outputs], ['text', 'usage'])
                Path(outputs[0].removeprefix("text=")).write_text(
                    '{"language":"zh-CN","segments":[]}', encoding="utf-8")

            with patch.object(dub_video, "model", side_effect=infer) as infer:
                with self.assertRaisesRegex(ValueError, "three attempts"):
                    dub_video.translate(str(manifest))
            self.assertEqual(infer.call_count, 3)
            self.assertTrue((root / "translation-response.txt").exists())
            self.assertEqual(dub_video.read_json(manifest)["stage"], "transcribed")

    def test_long_translation_keeps_every_segment_across_model_calls(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            manifest = self.manifest(root, stage="transcribed")
            value = dub_video.read_json(manifest)
            value["segments"] = [{"id": index, "start": index * 10, "end": index * 10 + 10,
                                  "text": "Hello"} for index in range(65)]
            value["duration_ms"] = 650
            dub_video.save_json(manifest, value)

            def infer(name, operation, inputs, outputs):
                prompt = Path(inputs[0].removeprefix("prompt=@"))
                request = json.loads(prompt.read_text(encoding="utf-8").split("\n")[-1])
                if prompt.name.startswith("translation-audit"):
                    self.assertTrue(all(item["source"] == "Hello" for item in request["segments"]))
                    reply = {"valid": True, "issues": []}
                else:
                    reply = {"language": "zh-CN", "segments": [{"id": item["id"], "text": "你好"}
                                                               for item in request["segments"]]}
                dub_video.save_json(Path(outputs[0].removeprefix("text=")), reply)

            with patch.object(dub_video, "model", side_effect=infer) as infer:
                dub_video.translate(str(manifest))
            self.assertEqual(infer.call_count, 4)
            result = dub_video.read_json(root / "translated-asr.json")["segments"]
            self.assertEqual([item["id"] for item in result], list(range(65)))
            self.assertEqual((result[-1]["start"], result[-1]["end"], result[-1]["text"]), (640, 650, "你好"))

    def test_semantic_rejection_retries_with_feedback_and_never_reaches_tts(self):
        issue = "Segment 7 reverses who helps whom: preserve YOU helping ME"
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            manifest = self.manifest(root, stage="transcribed")
            translation_prompts = []

            def infer(name, operation, inputs, outputs):
                self.assertEqual((name, operation), ("llm", "OMNI"))
                self.assertEqual([item.split("=", 1)[0] for item in outputs], ["text", "usage"])
                prompt = Path(inputs[0].removeprefix("prompt=@"))
                content = prompt.read_text(encoding="utf-8")
                if prompt.name.startswith("translation-audit"):
                    request = json.loads(content.split("\n")[-1])
                    self.assertEqual(request["segments"], [{"id": 7, "source": "Can you help?",
                                                           "translation": "我能帮您吗？"}])
                    reply = {"valid": False, "issues": [{"segment_id": 7, "suggested_correction": issue}]}
                else:
                    translation_prompts.append(content)
                    reply = {"language": "zh-CN", "segments": [{"id": 7, "text": "我能帮您吗？"}]}
                dub_video.save_json(Path(outputs[0].removeprefix("text=")), reply)

            with patch.object(dub_video, "model", side_effect=infer) as infer:
                with self.assertRaisesRegex(ValueError, "three attempts.*reverses"):
                    dub_video.translate(str(manifest))
                with self.assertRaisesRegex(ValueError, "requires validated"):
                    dub_video.synthesize(str(manifest))
            self.assertEqual(infer.call_count, 6)
            self.assertNotIn(issue, translation_prompts[0])
            self.assertTrue(all(issue in prompt for prompt in translation_prompts[1:]))
            for attempt in range(1, 4):
                audit = root / f"translation-audit-attempt-{attempt}.txt"
                self.assertEqual(dub_video.read_json(audit), {"valid": False, "issues": [
                    {"segment_id": 7, "suggested_correction": issue}]})
            self.assertEqual(dub_video.read_json(manifest)["stage"], "transcribed")
            self.assertFalse((root / "translated-asr.json").exists())

    def test_semantic_feedback_can_correct_translation_before_publication(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            manifest = self.manifest(root, stage="transcribed")
            attempts = 0

            def infer(name, operation, inputs, outputs):
                nonlocal attempts
                prompt = Path(inputs[0].removeprefix("prompt=@"))
                if prompt.name.startswith("translation-audit"):
                    reply = {"valid": attempts == 2, "issues": [] if attempts == 2 else ["Segment 7 reverses roles"]}
                else:
                    attempts += 1
                    if attempts == 2:
                        self.assertIn("Segment 7 reverses roles", prompt.read_text(encoding="utf-8"))
                    reply = {"language": "zh-CN", "segments": [{"id": 7,
                        "text": "我能帮您吗？" if attempts == 1 else "您能帮我吗？"}]}
                dub_video.save_json(Path(outputs[0].removeprefix("text=")), reply)

            with patch.object(dub_video, "model", side_effect=infer) as infer:
                dub_video.translate(str(manifest))
            self.assertEqual(infer.call_count, 4)
            self.assertEqual(dub_video.read_json(root / "translated-asr.json")["segments"][0]["text"], "您能帮我吗？")
            self.assertEqual(dub_video.read_json(manifest)["stage"], "translated")

    def test_audit_requires_strict_boolean_and_consistent_issue_schema(self):
        responses = ['{"valid":true,"issues":[],"extra":0}', '{"valid":"true","issues":[]}',
                     '{"valid":false,"issues":[]}', '{"valid":true,"issues":["bad"]}',
                     '{"valid":false,"issues":[""]}', '{"valid":false,"issues":[7]}',
                     '{"valid":false,"issues":[{"segment_id":8,"suggested_correction":"fix"}]}',
                     '{"valid":false,"issues":[{"segment_id":true,"suggested_correction":"fix"}]}',
                     '{"valid":false,"issues":[{"segment_id":7,"suggested_correction":""}]}',
                     '{"valid":false,"issues":[{"segment_id":7}]}',
                     '{"valid":false,"issues":[{"segment_id":7,"suggested_correction":"fix","extra":0}]}',
                     '{"valid":false,"valid":true,"issues":[]}', '{"filename":"audit.json"}',
                     '```json\n{"valid":true,"issues":[]}\n```\ncommentary',
                     '```json\n{"valid":true,"issues":[]}\n```\n```json\n{}\n```', '[]']
        translated = [{"id": 7, "source_text": "Can you help?", "text": "您能帮我吗？"}]
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for response in responses:
                with self.subTest(response=response):
                    def infer(name, operation, inputs, outputs):
                        prompt = Path(inputs[0].removeprefix("prompt=@")).read_text(encoding="utf-8")
                        self.assertIn("Determine intended context from the SOURCE segments", prompt)
                        self.assertIn("Never propose changing explicit play/resume music", prompt)
                        Path(outputs[0].removeprefix("text=")).write_text(response, encoding="utf-8")
                    with patch.object(dub_video, "model", side_effect=infer), self.assertRaises(ValueError):
                        dub_video.audit_translation(root, translated, "zh-CN", "llm", "", 1)

    def test_audit_accepts_single_json_object_or_one_enclosing_fence(self):
        translated = [{"id": 7, "source_text": "Can you help?", "text": "您能帮我吗？"}]
        payload = '{"valid":true,"issues":[]}'
        with tempfile.TemporaryDirectory() as directory:
            for response in [payload, "```json\n" + payload + "\n```", "```\n" + payload + "\n```"]:
                with self.subTest(response=response):
                    def infer(name, operation, inputs, outputs):
                        Path(outputs[0].removeprefix("text=")).write_text(response, encoding="utf-8")
                    with patch.object(dub_video, "model", side_effect=infer):
                        dub_video.audit_translation(Path(directory), translated, "zh-CN", "llm", "", 1)

    def test_pcm_timeline_preserves_gaps_and_does_not_wrap_riff_lengths(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            speech = root / "speech.pcm"
            speech.write_bytes(b"\x34\x12" * 480)
            destination = root / "timeline.pcm"
            dub_media.timeline([{"start": 40, "end": 60, "fitted_audio": str(speech)}], 100, destination)
            self.assertEqual(destination.read_bytes(), b"\0\0" * 960 + b"\x34\x12" * 480 + b"\0\0" * 960)
            self.assertEqual(destination.stat().st_size, 100 * 24 * 2)

    def test_too_long_speech_is_rejected_instead_of_cutting_words(self):
        with patch.object(dub_media, "probe", return_value={"format": {"duration": "12"}}), \
             patch.object(dub_media, "command") as run:
            with self.assertRaisesRegex(ValueError, "shorten its translation"):
                dub_media.fit_speech(Path("speech.wav"), Path("fitted.pcm"), {"id": 7, "start": 0, "end": 5000})
            run.assert_not_called()

    def test_truncated_pcm_fails_instead_of_publishing_silent_speech(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            speech = root / "speech.pcm"
            speech.write_bytes(b"\x01\x00")
            with self.assertRaisesRegex(ValueError, "wrong sample count"):
                dub_media.timeline([{"id": 7, "start": 40, "end": 60, "fitted_audio": str(speech)}],
                                   100, root / "timeline.pcm")

    def test_render_rejects_timing_changes_before_media_work(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            manifest = self.manifest(root, stage="synthesized")
            data = dub_video.read_json(root / "translated.json")
            data["segments"][0]["start"] = 41
            dub_video.save_json(root / "translated.json", data)
            with patch.object(dub_video, "timeline") as assemble:
                with self.assertRaisesRegex(ValueError, "changed after synthesis"):
                    dub_video.render(str(manifest))
            assemble.assert_not_called()
            self.assertFalse((root / "dubbed.mp4").exists())

    def test_publication_preserves_old_video_on_required_failure(self):
        for failed_write in (1, 2):
            with self.subTest(failed_write=failed_write), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                manifest = self.manifest(root, stage="synthesized")
                data = dub_video.read_json(manifest)
                data.update(source_ass="", asr="source-asr.json", transcript="source.txt",
                            output=str(root / "customer.mp4"))
                dub_video.save_json(manifest, data)
                output = root / "customer.mp4"
                output.write_bytes(b"previous video")
                writes = 0
                original_save = dub_video.save_json

                def save(path, value):
                    nonlocal writes
                    writes += 1
                    if writes == failed_write:
                        raise OSError("metadata disk full")
                    original_save(path, value)

                def assemble(segments, duration, destination):
                    destination.write_bytes(b"pcm")

                def render(video, audio, subtitles, destination, language):
                    destination.write_bytes(b"verified dubbed video")

                with patch.object(dub_video, "save_json", side_effect=save), \
                     patch.object(dub_video, "timeline", side_effect=assemble), \
                     patch.object(dub_video, "mux", side_effect=render):
                    if failed_write == 1:
                        with self.assertRaisesRegex(OSError, "disk full"):
                            dub_video.render(str(manifest))
                        self.assertEqual(output.read_bytes(), b"previous video")
                    else:
                        result = dub_video.render(str(manifest))
                        self.assertEqual(result["video"], str(output))
                        self.assertEqual(output.read_bytes(), b"verified dubbed video")

    def test_mux_uses_container_supported_subtitles_and_preserves_video(self):
        streams = {"streams": [{"codec_type": kind} for kind in ("video", "audio", "subtitle")]}
        for extension, codec in (("mp4", "mov_text"), ("mkv", "ass")):
            with self.subTest(extension=extension), patch.object(dub_media, "command") as run, \
                 patch.object(dub_media, "probe", return_value=streams):
                dub_media.mux(Path("source.mp4"), Path("timeline.pcm"), Path("中文 subtitles.ass"),
                              Path("dubbed." + extension), "zh-CN")
                argv = run.call_args.args[0]
                self.assertEqual(argv[argv.index("-c:v") + 1], "copy")
                self.assertEqual(argv[argv.index("-c:s") + 1], codec)
                self.assertEqual(argv.count("language=" + ("zho" if extension == "mp4" else "zh-CN")), 2)
                self.assertIn("中文 subtitles.ass", argv)
                self.assertIn("s16le", argv)


if __name__ == "__main__":
    unittest.main()
