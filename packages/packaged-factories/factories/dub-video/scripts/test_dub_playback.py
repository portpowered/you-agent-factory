"""Speech may occupy only the source cue and its unoccupied trailing gap."""

import copy
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from dub_contract import playback_segments, render_ass, render_srt
from dub_media import fit_speech, timeline


class DubPlaybackTests(unittest.TestCase):
    def test_short_speech_borrows_only_needed_gap_without_changing_source(self):
        segment = {"id": 16, "start": 65120, "end": 65440, "text": "This is,",
                   "source_text": "这是，", "reference_sha256": "original"}
        original = copy.deepcopy(segment)
        with patch("dub_media.probe", return_value={"format": {"duration": "1.120"}}), \
             patch("dub_media.command") as run:
            self.assertEqual(fit_speech(Path("speech.wav"), Path("speech.pcm"), segment, 67200), 1.0)
        self.assertEqual(segment, {**original, "speech_end": 66240})
        self.assertIn("atempo=1.00000000,apad,atrim=duration=1.120", run.call_args.args[0])

    def test_next_cue_and_video_end_keep_two_times_speed_guard(self):
        for limit in (65440, 65600):  # No gap, or insufficient gap before a next cue/video end.
            segment = {"id": 16, "start": 65120, "end": 65440}
            with self.subTest(limit=limit), \
                 patch("dub_media.probe", return_value={"format": {"duration": "1.120"}}), \
                 patch("dub_media.command") as run:
                with self.assertRaisesRegex(ValueError, "shorten its translation"):
                    fit_speech(Path("speech.wav"), Path("speech.pcm"), segment, limit)
                run.assert_not_called()
                self.assertNotIn("speech_end", segment)

    def test_last_cue_uses_only_video_tail_and_short_speech_keeps_source_end(self):
        for duration, expected, speed in (("0.030", 70, 1.0), ("0.010", 60, 1.0), ("0.100", 100, 100 / 60)):
            segment = {"id": 7, "start": 40, "end": 60, "text": "Hi"}
            with self.subTest(duration=duration), \
                 patch("dub_media.probe", return_value={"format": {"duration": duration}}), \
                 patch("dub_media.command"):
                self.assertAlmostEqual(fit_speech(Path("s.wav"), Path("s.pcm"), segment, 100), speed)
                self.assertEqual(segment["speech_end"], expected)
                self.assertEqual(playback_segments([segment], 100)[0]["end"], expected)

    def test_playback_rejects_overlap_invalid_end_and_video_overrun(self):
        source = [{"id": 1, "start": 0, "end": 20, "text": "One"},
                  {"id": 2, "start": 40, "end": 60, "text": "Two"}]
        for index, end in ((0, 41), (0, 19), (0, True), (1, 101)):
            segments = copy.deepcopy(source)
            segments[index]["speech_end"] = end
            with self.subTest(index=index, end=end), self.assertRaisesRegex(ValueError, "trailing silent gap"):
                playback_segments(segments, 100)

    def test_pcm_and_captions_share_playback_windows_preserving_source_evidence(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            speech = root / "speech.pcm"
            speech.write_bytes(b"\x34\x12" * 720)
            source = [{"id": 7, "start": 40, "end": 60, "speech_end": 70,
                       "text": "Hello", "fitted_audio": str(speech)}]
            playback = playback_segments(source, 100)
            timeline(playback, 100, root / "timeline.pcm")
            self.assertEqual((root / "timeline.pcm").read_bytes(),
                             b"\0\0" * 960 + b"\x34\x12" * 720 + b"\0\0" * 720)
            self.assertIn("00:00:00,040 --> 00:00:00,070", render_srt(playback))
            self.assertIn("0:00:00.04,0:00:00.07", render_ass(playback))
            self.assertEqual(source[0]["end"], 60)


if __name__ == "__main__":
    unittest.main()
