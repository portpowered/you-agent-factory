"""Only matched nonlexical phonetic cues preserve the exact source interval."""

import struct
import tempfile
import unittest
import wave
from pathlib import Path
from unittest.mock import patch

import dub_video
import dub_media
from dub_contract import nonverbal_kind, render_srt
from dub_media import CommandFailed, preserve_nonverbal, timeline


class DubNonverbalTests(unittest.TestCase):
    def test_conservative_whole_cue_inventory_across_five_languages(self):
        cries = [("啊啊！", "Ahhh!"), ("あああ！", "啊啊！"), ("아아아!", "Ahhh!"),
                 ("Ahhh!", "アアア！"), ("¡Ahhh!", "아아아!")]
        laughs = [("哈哈哈！", "Hahaha!"), ("ハハハ！", "Jajaja!"), ("하하하!", "Hahaha!"),
                  ("Hahaha!", "哈哈哈！"), ("¡Jajaja!", "하하하!")]
        for kind, pairs in [("cry", cries), ("laugh", laughs)]:
            for source, target in pairs:
                with self.subTest(source=source, target=target):
                    self.assertEqual(nonverbal_kind(source, target), kind)

    def test_single_interjections_words_annotations_mixed_kinds_and_names_do_not_match(self):
        for source, target in [("啊", "Ahhh"), ("ああ", "Ahhh"), ("아아", "Ahhh"),
                ("Ah", "啊啊"), ("Ha", "哈哈哈"), ("嗯", "Yes"), ("Wow", "啊啊"),
                ("Oh", "啊啊"), ("No", "哈哈哈"), ("哈哈哈谢谢", "Hahaha"),
                ("あああ助けて", "Ahhh"), ("아아아 도와줘", "Ahhh"),
                ("Ahhh help me!", "啊啊"), ("Jajaja gracias", "哈哈哈"),
                ("啊啊", "Ahhh help me"), ("[laughter]", "Hahaha"),
                ("(screaming)", "Ahhh"), ("啊啊", "Hahaha"), ("哈 哈 哈", "Hahaha")]:
            with self.subTest(source=source, target=target):
                self.assertIsNone(nonverbal_kind(source, target))
        self.assertIsNone(nonverbal_kind("Hahaha", "哈哈哈", ["Hahaha"]))

    def test_hum_is_matched_only_when_both_cue_sides_are_pure_affirmative_hums(self):
        for source, target in [("嗯。", "Mm."), ("嗯", "Mm"), ("嗯嗯嗯！", "Mmm!"), ("嗯嗯", "mm"),
                               ("Mm.", "Mm."), ("嗯嗯", "嗯嗯")]:
            with self.subTest(source=source, target=target):
                self.assertEqual(nonverbal_kind(source, target), "hum")
        # Spoken affirmations and lexical or annotated content stay translated speech.
        for source, target in [("嗯。", "Yes."), ("嗯。", "No."), ("嗯。", "Okay."), ("嗯", "I see"),
                               ("嗯嗯", "Mmhm"), ("嗯嗯", "Hmm"), ("嗯嗯", "Hmm-mm"), ("嗯", "M"),
                               ("嗯嗯", "m"), ("嗯 我同意", "Mm."), ("嗯。", "Mm, sure"),
                               ("[thinking]", "Mm."), ("(agreement)", "Mm."), ("Mm.", "Yes."),
                               ("嗯 嗯", "Mm."), ("Mm", "Mm Mm"), ("嗯嗯嗯", "Mm, yes")]:
            with self.subTest(source=source, target=target):
                self.assertIsNone(nonverbal_kind(source, target))
        # A source cue carrying a preserved name, and another nonverbal kind, never become a hum.
        self.assertIsNone(nonverbal_kind("Mmm.", "Mm.", ["Mmm"]))
        self.assertIsNone(nonverbal_kind("嗯。", "Hahaha."))
        self.assertIsNone(nonverbal_kind("Mmm.", "Ahhh!"))
        self.assertIsNone(nonverbal_kind("嗯嗯", "哈哈哈"))

    def write_wave(self, path, frames=24000):
        samples = struct.pack("<h", 1234) * frames
        with wave.open(str(path), "wb") as output:
            output.setparams((1, 2, 24000, 0, "NONE", "not compressed"))
            output.writeframes(samples)
        return samples

    def decode_wave_command(self, argv):
        """Controlled command edge; native FFmpeg is a separate artifact proof."""
        source = Path(argv[argv.index("-i") + 1])
        destination = Path(argv[-1])
        with wave.open(str(source), "rb") as input_wave:
            self.assertEqual((input_wave.getnchannels(), input_wave.getsampwidth(), input_wave.getframerate()),
                             (1, 2, 24000))
            destination.write_bytes(input_wave.readframes(input_wave.getnframes()))

    def test_source_audio_pcm_is_exact_with_original_bounds_and_no_tts(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            segment = {"id": 0, "start": 1000, "end": 2000, "source_text": "啊啊！", "text": "Ahhh!"}
            source = dict(segment)
            samples = []

            def reference(video, original, destination):
                self.assertEqual(original, source)
                samples.append(self.write_wave(destination))

            value = {"video": str(root / "video.mp4"), "duration_ms": 5000,
                     "language": "en-US", "models": {"tts": "tts", "llm": "llm"}}
            with patch.object(dub_video, "reference", side_effect=reference), \
                 patch.object(dub_media, "command", side_effect=self.decode_wave_command), \
                 patch.object(dub_video, "model") as model:
                output = dub_video.synthesize_segment(root, [segment], 0, value)
            model.assert_not_called()
            self.assertEqual(segment, source)
            self.assertEqual(Path(output["fitted_audio"]).read_bytes(), samples[0])
            self.assertEqual(output["speech_end"], 2000)
            self.assertEqual(output["speech_speed"], 1.0)
            self.assertEqual(output["audio_origin"], "source-nonverbal")
            self.assertEqual(output["nonverbal_kind"], "cry")
            self.assertEqual(output["tts_attempts"], [])
            self.assertEqual(output["speech_audio"], output["reference_audio"])
            self.assertEqual(output["reference_sha256"], dub_video.digest(Path(output["reference_audio"])))
            timeline_file = root / "timeline.pcm"
            timeline([output], 5000, timeline_file)
            expected = bytes(1000 * 48) + samples[0] + bytes(3000 * 48)
            self.assertEqual(timeline_file.read_bytes(), expected)
            self.assertIn("00:00:01,000 --> 00:00:02,000", render_srt([output]))

    def test_hum_cue_preserves_source_pcm_with_original_bounds_and_no_tts(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            # The production failure: a lone 嗯。 rendered as Mm. exhausted every TTS attempt.
            segment = {"id": 45, "start": 178000, "end": 179520,
                       "source_text": "嗯。", "text": "Mm."}
            source = dict(segment)
            samples = []

            def reference(video, original, destination):
                self.assertEqual(original, source)
                samples.append(self.write_wave(destination, (original["end"] - original["start"]) * 24))

            value = {"video": str(root / "video.mp4"), "duration_ms": 180000,
                     "language": "en-US", "models": {"tts": "tts", "llm": "llm"}}
            with patch.object(dub_video, "reference", side_effect=reference), \
                 patch.object(dub_media, "command", side_effect=self.decode_wave_command), \
                 patch.object(dub_video, "model") as model:
                output = dub_video.synthesize_segment(root, [segment], 0, value)
            model.assert_not_called()
            self.assertEqual(segment, source)
            self.assertEqual(output["nonverbal_kind"], "hum")
            self.assertEqual(output["audio_origin"], "source-nonverbal")
            self.assertEqual((output["start"], output["end"], output["speech_end"]), (178000, 179520, 179520))
            self.assertEqual(output["speech_speed"], 1.0)
            self.assertEqual(output["tts_attempts"], [])
            self.assertEqual(output["fit_attempts"], [{"revision": 0, "status": "fitted"}])
            self.assertEqual(output["speech_audio"], output["reference_audio"])
            self.assertEqual(Path(output["fitted_audio"]).read_bytes(), samples[0])
            self.assertEqual(len(samples[0]), (179520 - 178000) * 48)
            self.assertEqual(output["reference_sha256"], dub_video.digest(Path(output["reference_audio"])))
            timeline_file = root / "timeline.pcm"
            timeline([output], 180000, timeline_file)
            self.assertEqual(timeline_file.read_bytes(),
                             bytes(178000 * 48) + samples[0] + bytes((180000 - 179520) * 48))
            self.assertIn("00:02:58,000 --> 00:02:59,520", render_srt([output]))

    def test_mixed_lexical_cue_never_becomes_a_backend_error_fallback(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            segment = {"id": 0, "start": 0, "end": 1000, "source_text": "啊啊帮我！", "text": "Ahhh help me!"}
            value = {"video": "video.mp4", "duration_ms": 1000, "language": "en-US", "models": {"tts": "tts"}}
            failure = CommandFailed("you", 1, "unclassified backend failure")
            with patch.object(dub_video, "reference", side_effect=lambda v, s, d: self.write_wave(d)), \
                 patch.object(dub_video, "model", side_effect=failure) as model, \
                 patch.object(dub_video, "preserve_nonverbal") as preserve:
                with self.assertRaises(CommandFailed) as caught:
                    dub_video.synthesize_segment(root, [segment], 0, value)
            self.assertIs(caught.exception, failure)
            self.assertEqual(model.call_count, 1)
            preserve.assert_not_called()

    def test_reference_length_mismatch_fails_instead_of_padding_or_truncating(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            ref = root / "reference.wav"
            self.write_wave(ref)
            segment = {"id": 0, "start": 0, "end": 500}
            with patch.object(dub_media, "command", side_effect=self.decode_wave_command):
                with self.assertRaisesRegex(ValueError, "original interval"):
                    preserve_nonverbal(ref, root / "output.pcm", segment)
            self.assertNotIn("speech_end", segment)


if __name__ == "__main__":
    unittest.main()
