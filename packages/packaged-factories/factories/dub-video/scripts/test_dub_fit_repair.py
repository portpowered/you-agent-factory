"""Measured speech overflow permits only bounded, audited concise revisions."""

import copy
import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import dub_video
from dub_media import SpeechDoesNotFit


class DubFitRepairTests(unittest.TestCase):
    def setUp(self):
        # These tests isolate the existing decision/fit contract. The separate
        # two-stage audit suite exercises the real comparison dispatch.
        comparison = patch.object(dub_video, "compare_translation", return_value="Source comparison evidence")
        comparison.start()
        self.addCleanup(comparison.stop)

    def test_indexed_correction_is_reaudited_before_replacement_speech(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            translated, value = self.inputs(root)
            calls = []

            def infer(name, operation, inputs, outputs, parameters=None, server=""):
                if operation == "TTS":
                    calls.append(("tts", inputs[0]))
                    Path(outputs[0].removeprefix("audio=")).write_bytes(b"speech")
                    return
                self.assertEqual(set(parameters), {"json_schema"})
                self.assertEqual(parameters["json_schema"]["type"], "object")
                prompt = Path(inputs[0].removeprefix("prompt=@"))
                request = json.loads(prompt.read_text(encoding="utf-8").split("\n")[-1])
                if prompt.name.startswith("fit-translation"):
                    reply = {"language": "en-US", "segments": [{"id": 1, "text": "Can anyone help me?"}]}
                else:
                    text = request["segments"][1]["translation"]
                    calls.append(("audit", text))
                    reply = {"valid": text == "Can anyone kind help me?", "issues": []}
                    if not reply["valid"]:
                        reply["issues"] = [{"segment_id": 1, "suggested_correction": "Can anyone kind help me?"}]
                dub_video.save_json(Path(outputs[0].removeprefix("text=")), reply)

            def fit(speech, fitted, segment, limit):
                if segment["text"].startswith("Is there"):
                    raise SpeechDoesNotFit(1, 2320, 1120, 2.07)
                fitted.write_bytes(b"fitted")
                segment["speech_end"] = 2120
                return 1.64

            with patch.object(dub_video, "reference", side_effect=lambda v, s, d: d.write_bytes(b"reference")), \
                 patch.object(dub_video, "model", side_effect=infer), \
                 patch.object(dub_video, "fit_speech", side_effect=fit):
                result = dub_video.synthesize_segment(root, translated, 1, value)
            self.assertEqual(calls, [("tts", "text=Is there anyone kind who can help me?"),
                ("audit", "Can anyone help me?"), ("audit", "Can anyone kind help me?"),
                ("tts", "text=Can anyone kind help me?")])
            self.assertEqual(result["text"], "Can anyone kind help me?")
            self.assertEqual(result["audio_origin"], "reference-conditioned")
            self.assertEqual(dub_video.read_json(root / "fit-translation-1-revision-1-approval.json")["audit_attempt"], 2)
            self.assertTrue((root / "fit-translation-1-revision-1-candidate-attempt-1.json").is_file())
            self.assertTrue((root / "fit-translation-1-revision-1-audit-rejection-1.json").is_file())

    def inputs(self, root):
        source = [{"id": 0, "start": 0, "end": 900, "text": "你好"},
                  {"id": 1, "start": 1000, "end": 2120, "text": "有没有好心人可以帮帮我？"},
                  {"id": 2, "start": 2120, "end": 3000, "text": "拜托拜托！"}]
        texts = ["Hello", "Is there anyone kind who can help me?", "Please, please!"]
        translated = [{**item, "source_text": item["text"], "text": text} for item, text in zip(source, texts)]
        value = {"segments": source, "video": str(root / "original.mp4"), "duration_ms": 3000,
                 "language": "en-US", "models": {"tts": "tts", "llm": "llm"}}
        return translated, value

    def test_measured_overflow_revises_only_cue_after_complete_semantic_audit(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            translated, value = self.inputs(root)
            original = copy.deepcopy(translated)
            calls, extracted, fit_calls = [], [], []

            def extract(video, segment, destination):
                extracted.append((segment["id"], segment["start"], segment["end"]))
                destination.write_bytes(b"same original voice")

            def infer(name, operation, inputs, outputs, parameters=None, server=""):
                if operation == "TTS":
                    calls.append(("tts", inputs[0]))
                    self.assertEqual(Path(inputs[1].removeprefix("voice=@")).read_bytes(), b"same original voice")
                    self.assertEqual(parameters, {"language": "English"})
                    Path(outputs[0].removeprefix("audio=")).write_bytes(b"generated speech")
                    return
                prompt = Path(inputs[0].removeprefix("prompt=@"))
                request = json.loads(prompt.read_text(encoding="utf-8").split("\n")[-1])
                if prompt.name.startswith("fit-translation"):
                    calls.append(("repair", request["segment_id"]))
                    self.assertEqual((request["generated_ms"], request["available_ms"]), (2320, 1120))
                    self.assertEqual([item["source"] for item in request["context"]], [s["text"] for s in value["segments"]])
                    reply = {"language": "en-US", "segments": [{"id": 1, "text": "Can anyone kind help me?"}]}
                else:
                    calls.append(("audit", [item["id"] for item in request["segments"]]))
                    self.assertEqual(request["segments"][0]["translation"], "Hello")
                    self.assertEqual(request["segments"][2]["translation"], "Please, please!")
                    reply = {"valid": True, "issues": []}
                dub_video.save_json(Path(outputs[0].removeprefix("text=")), reply)

            def fit(speech, fitted, segment, limit):
                fit_calls.append(limit)
                if len(fit_calls) == 1:
                    raise SpeechDoesNotFit(1, 2320, 1120, 2320 / 1120)
                segment["speech_end"] = 2120
                fitted.write_bytes(b"fitted speech")
                return 1.75

            with patch.object(dub_video, "reference", side_effect=extract), \
                 patch.object(dub_video, "model", side_effect=infer), \
                 patch.object(dub_video, "fit_speech", side_effect=fit):
                result = dub_video.synthesize_segment(root, translated, 1, value)
            self.assertEqual(calls, [("tts", "text=Is there anyone kind who can help me?"),
                ("repair", 1), ("audit", [0, 1, 2]), ("tts", "text=Can anyone kind help me?")])
            self.assertEqual(extracted, [(1, 1000, 2120)])
            self.assertEqual(fit_calls, [2120, 2120])
            self.assertEqual(translated, original)
            self.assertEqual((result["id"], result["start"], result["end"], result["source_text"]),
                             (1, 1000, 2120, original[1]["source_text"]))
            self.assertEqual(result["reference_sha256"], dub_video.digest(Path(result["reference_audio"])))
            self.assertEqual(result["text"], "Can anyone kind help me?")
            self.assertEqual(len(result["fit_attempts"]), 2)

    def test_semantic_rejection_never_generates_rejected_text_and_is_bounded(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            translated, value = self.inputs(root)
            tts_calls, repairs, audits = [], [], []

            def infer(name, operation, inputs, outputs, parameters=None, server=""):
                if operation == "TTS":
                    tts_calls.append(inputs[0])
                    Path(outputs[0].removeprefix("audio=")).write_bytes(b"speech")
                    return
                prompt = Path(inputs[0].removeprefix("prompt=@"))
                request = json.loads(prompt.read_text(encoding="utf-8").split("\n")[-1])
                if prompt.name.startswith("fit-translation"):
                    repairs.append(request)
                    reply = {"language": "en-US", "segments": [{"id": 1, "text": "I can help you."}]}
                else:
                    audits.append(request)
                    reply = {"valid": False, "issues": [{"segment_id": 1,
                        "suggested_correction": "Is there anyone kind who can help me?"}]}
                dub_video.save_json(Path(outputs[0].removeprefix("text=")), reply)

            def extract(video, segment, destination):
                destination.write_bytes(b"original voice")

            with patch.object(dub_video, "reference", side_effect=extract), \
                 patch.object(dub_video, "model", side_effect=infer), \
                 patch.object(dub_video, "fit_speech", side_effect=SpeechDoesNotFit(1, 2320, 1120, 2.07)):
                with self.assertRaises(dub_video.FitTranslationRejected):
                    dub_video.synthesize_segment(root, translated, 1, value)
            self.assertEqual(len(tts_calls), 1)
            self.assertEqual((len(repairs), len(audits)), (2, 6))
            self.assertIn("Is there anyone kind who can help me?", repairs[1]["previous_rejection"])
            self.assertEqual(len(list(root.glob("*.fit-revision-*-rejected.json"))), 2)
            self.assertEqual(translated[1]["text"], "Is there anyone kind who can help me?")

    def test_three_fitted_versions_fail_without_publishing_changed_translation(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            translated, value = self.inputs(root)
            original = copy.deepcopy(translated)
            calls = []

            def infer(name, operation, inputs, outputs, parameters=None, server=""):
                calls.append(operation)
                if operation == "TTS":
                    Path(outputs[0].removeprefix("audio=")).write_bytes(b"too much speech")
                    return
                prompt = Path(inputs[0].removeprefix("prompt=@"))
                reply = {"valid": True, "issues": []} if prompt.name.startswith("translation-audit") else \
                    {"language": "en-US", "segments": [{"id": 1, "text": "Can anyone kind help me?"}]}
                dub_video.save_json(Path(outputs[0].removeprefix("text=")), reply)

            def extract(video, segment, destination):
                destination.write_bytes(b"original")

            with patch.object(dub_video, "reference", side_effect=extract), \
                 patch.object(dub_video, "model", side_effect=infer), \
                 patch.object(dub_video, "fit_speech", side_effect=SpeechDoesNotFit(1, 3000, 1120, 2.68)):
                with self.assertRaises(SpeechDoesNotFit):
                    dub_video.synthesize_segment(root, translated, 1, value)
            self.assertEqual(calls.count("TTS"), 3)
            self.assertEqual(calls.count("OMNI"), 4)
            self.assertEqual(len(list(root.glob("*.fit-failure.json"))), 3)
            self.assertEqual(translated, original)

    def test_generic_fit_error_is_not_a_translation_retry(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            translated, value = self.inputs(root)
            with patch.object(dub_video, "reference", side_effect=lambda v, s, d: d.write_bytes(b"voice")), \
                 patch.object(dub_video, "model") as infer, \
                 patch.object(dub_video, "fit_speech", side_effect=ValueError("ffprobe failed")):
                with self.assertRaisesRegex(ValueError, "ffprobe failed"):
                    dub_video.synthesize_segment(root, translated, 1, value)
            self.assertEqual(infer.call_count, 1)

    def test_long_candidate_is_validated_and_saved_while_audit_keeps_local_context(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = [{"id": index, "start": index * 1000, "end": index * 1000 + 500,
                       "text": "Hello there"} for index in range(65)]
            translated = [{**item, "source_text": item["text"]} for item in source]
            value = {"segments": source, "language": "en-US", "models": {"llm": "llm"}}
            audits = []

            def infer(name, operation, inputs, outputs, parameters=None):
                prompt = Path(inputs[0].removeprefix("prompt=@"))
                request = json.loads(prompt.read_text(encoding="utf-8").split("\n")[-1])
                if prompt.name.startswith("translation-audit"):
                    audits.append(request)
                    reply = {"valid": True, "issues": []}
                else:
                    self.assertEqual([item["id"] for item in request["context"]], list(range(30, 35)))
                    reply = {"language": "en-US", "segments": [{"id": 32, "text": "Hello"}]}
                dub_video.save_json(Path(outputs[0].removeprefix("text=")), reply)

            with patch.object(dub_video, "model", side_effect=infer):
                repaired = dub_video.repair_fit_translation(root, translated, 32, value, 1,
                                                           SpeechDoesNotFit(32, 1500, 1000, 1.5))
            self.assertEqual(repaired["text"], "Hello")
            self.assertEqual([item["id"] for item in audits[0]["segments"]], list(range(30, 35)))
            saved = dub_video.read_json(root / "fit-translation-32-revision-1-candidate.json")["segments"]
            self.assertEqual(len(saved), 65)
            self.assertEqual([item["text"] for item in saved if item["id"] != 32], ["Hello there"] * 64)
            self.assertEqual(translated[32]["text"], "Hello there")

    def test_invalid_array_is_rejected_before_schema_guided_object_and_audit(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            translated, value = self.inputs(root)
            requests = []

            def infer(name, operation, inputs, outputs, parameters=None):
                prompt = Path(inputs[0].removeprefix("prompt=@"))
                content = prompt.read_text(encoding="utf-8")
                if prompt.name.startswith("fit-translation"):
                    request = json.loads(content.splitlines()[-1])
                    requests.append(request)
                    example = next(json.loads(line) for line in content.splitlines()
                                   if line.startswith('{"language"') and '"segments"' in line)
                    self.assertEqual(set(example), {"language", "segments"})
                    self.assertEqual(example["language"], "en-US")
                    self.assertEqual(example["segments"][0]["id"], 1)
                    self.assertEqual(set(example["segments"][0]), {"id", "text"})
                    self.assertIn("never an outer array", content)
                    self.assertIn("Input metadata is not the response schema", content)
                    if len(requests) == 1:
                        reply = [{"language": "en-US", "segment_id": 1,
                                  "replacement_text": "Can someone kind help me?"}]
                    else:
                        self.assertIn("JSON object", request["previous_rejection"])
                        example["segments"][0]["text"] = "Can someone kind help me?"
                        reply = example
                else:
                    reply = {"valid": True, "issues": []}
                dub_video.save_json(Path(outputs[0].removeprefix("text=")), reply)

            overflow = SpeechDoesNotFit(1, 2320, 1120, 2.07)
            with patch.object(dub_video, "model", side_effect=infer):
                with self.assertRaises(dub_video.FitTranslationRejected) as rejected:
                    dub_video.repair_fit_translation(root, translated, 1, value, 1, overflow)
                repaired = dub_video.repair_fit_translation(root, translated, 1, value, 2, overflow, str(rejected.exception))
            self.assertEqual(repaired["text"], "Can someone kind help me?")
            self.assertFalse((root / "fit-translation-1-revision-1-candidate.json").exists())
            self.assertTrue((root / "fit-translation-1-revision-2-candidate.json").exists())


if __name__ == "__main__":
    unittest.main()
