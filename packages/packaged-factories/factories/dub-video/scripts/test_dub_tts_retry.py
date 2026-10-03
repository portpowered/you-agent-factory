"""Only classified native EOS exhaustion can repeat an unchanged TTS request."""

import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import dub_video
from dub_media import CommandFailed


def exhaustion():
    return CommandFailed("you", 1, json.dumps({"code": "MODEL_BACKEND_FAILURE",
        "message": "TTS generation limit reached without EOS"}))


class DubTTSRetryTests(unittest.TestCase):
    def test_exhaustion_retry_keeps_text_language_reference_and_attempt_evidence(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            ref = root / "source.reference.wav"
            ref.write_bytes(b"original source audio")
            segment = {"id": 0, "start": 5920, "end": 13440, "text": "Ahhh!"}
            calls = []

            def infer(name, operation, inputs, outputs, parameters, server):
                calls.append((name, operation, inputs, parameters, server, ref.read_bytes()))
                audio = Path(outputs[0].removeprefix("audio="))
                audio.write_bytes(b"failed staging" if len(calls) == 1 else b"complete speech")
                if len(calls) == 1:
                    raise exhaustion()

            with patch.object(dub_video, "model", side_effect=infer):
                speech = dub_video.synthesize_speech(root / "segment-0", segment, ref,
                    "qwen3-tts-base", "en-US", "configured-server")
            self.assertEqual(len(calls), 2)
            self.assertEqual(calls[0], calls[1])
            self.assertEqual(calls[1][2], ["text=Ahhh!", f"voice=@{ref}"])
            self.assertEqual(calls[1][3], {"language": "English"})
            self.assertEqual(speech.read_bytes(), b"complete speech")
            self.assertEqual(speech.name, "segment-0.speech-attempt-2.wav")
            self.assertEqual((root / "segment-0.speech.wav").read_bytes(), b"failed staging")
            evidence = dub_video.read_json(Path(segment["tts_attempts"][0]["evidence"]))
            self.assertEqual(evidence["reason"], "generation-exhausted-without-eos")
            self.assertEqual(segment["tts_attempts"][1]["status"], "succeeded")
            self.assertEqual((segment["start"], segment["end"]), (5920, 13440))

    def test_three_exhaustions_stop_with_all_failures_retained(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            segment = {"id": 7, "text": "Hello"}
            with patch.object(dub_video, "model", side_effect=exhaustion()) as infer:
                with self.assertRaises(CommandFailed):
                    dub_video.synthesize_speech(root / "segment-7", segment, root / "ref.wav", "tts", "en", "")
            self.assertEqual(infer.call_count, 3)
            self.assertEqual(len(list(root.glob("*-failure.json"))), 3)
            self.assertNotIn("tts_attempts", segment)

    def test_other_failures_and_cancellation_propagate_without_retry(self):
        errors = [ValueError("bad model response"), KeyboardInterrupt(),
                  CommandFailed("you", 1, '{"code":"CANCELLED","message":"cancelled"}'),
                  CommandFailed("you", 130, exhaustion().detail),
                  CommandFailed("you", 1, "TTS generation limit reached without EOS"),
                  CommandFailed("you", 1, '{"code":"MODEL_BACKEND_FAILURE","message":"other failure"}')]
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for error in errors:
                with self.subTest(error=error), patch.object(dub_video, "model", side_effect=error) as infer:
                    with self.assertRaises(type(error)):
                        dub_video.synthesize_speech(root / "segment-7", {"text": "Hello"},
                            root / "ref.wav", "tts", "en", "")
                    self.assertEqual(infer.call_count, 1)
            self.assertEqual(list(root.iterdir()), [])

    def test_latest_structured_failure_controls_retry_classification(self):
        detail = exhaustion().detail + '\n{"code":"CANCELLED","message":"cancelled"}'
        self.assertFalse(dub_video.tts_exhausted(CommandFailed("you", 1, detail)))
        self.assertTrue(dub_video.tts_exhausted(CommandFailed("you", 1, "status\n" + exhaustion().detail)))


if __name__ == "__main__":
    unittest.main()
