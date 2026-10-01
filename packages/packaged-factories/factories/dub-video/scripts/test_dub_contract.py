import copy
import json
import tempfile
import unittest
from pathlib import Path

from dub_contract import (
    load_translation_response, render_ass, render_srt, translation_prompt,
    validate_segments, validate_translations,
)


class DubContractTests(unittest.TestCase):
    def setUp(self):
        self.source = [{"id": 0, "start": 1, "end": 1001, "text": "Hello everyone"},
                       {"id": 3, "start": 1001, "end": 2500, "text": "How are you?"}]
        self.translated = {"language": "zh-CN", "segments": [
            {"id": 0, "text": "大家好"}, {"id": 3, "text": "你们好吗？"}]}

    def test_segments_preserve_source_and_reject_invalid_timelines(self):
        self.assertEqual(validate_segments(self.source, 2500), self.source)
        for field, value in [("id", True), ("id", -1), ("start", False), ("start", -1),
                             ("end", 1.5), ("end", 1), ("text", " "), ("text", None)]:
            with self.subTest(field=field, value=value):
                source = copy.deepcopy(self.source)
                source[0][field] = value
                with self.assertRaises(ValueError):
                    validate_segments(source)
        for source in [[], None, {}, self.source[::-1], [self.source[0], self.source[0]]]:
            with self.assertRaises(ValueError):
                validate_segments(source)
        overlap = copy.deepcopy(self.source)
        overlap[1]["start"] = 1000
        with self.assertRaises(ValueError):
            validate_segments(overlap)
        for duration in [True, 2499, 0, 1.5]:
            with self.assertRaises(ValueError):
                validate_segments(self.source, duration)

    def test_translations_keep_exact_ids_timestamps_source_and_unicode(self):
        result = validate_translations(self.translated, self.source, "zh-CN")
        self.assertEqual(result[0], {"id": 0, "start": 1, "end": 1001,
                                    "source_text": "Hello everyone", "text": "大家好"})
        self.assertEqual(result[1]["end"], 2500)
        self.assertIn('"language": "zh-CN"', translation_prompt(self.source, "zh-CN"))
        self.assertIn("timestamps", translation_prompt(self.source, "zh-CN"))
        music_source = [{"id": 7, "start": 0, "end": 1000, "text": "Please play silver clouds."},
                        {"id": 8, "start": 1000, "end": 2000, "text": "Let us listen to music."}]
        music_prompt = translation_prompt(music_source, "zh-CN")
        request = json.loads(music_prompt.split("\n")[-1])
        self.assertEqual(request["segments"], [{"id": item["id"], "text": item["text"]} for item in music_source])
        self.assertIn("Keep the explicit play/resume music action", music_prompt)
        self.assertIn("ASR lowercases", music_prompt)

    def test_translations_reject_count_order_duplicate_new_ids_and_extra_fields(self):
        mutations = [lambda v: v.update(language="en"), lambda v: v.update(extra=True),
                     lambda v: v["segments"].pop(), lambda v: v["segments"].reverse(),
                     lambda v: v["segments"][1].update(id=0), lambda v: v["segments"][1].update(id=9),
                     lambda v: v["segments"][0].update(id=False),
                     lambda v: v["segments"][0].update(start=99),
                     lambda v: v["segments"][0].update(text=" ")]
        for mutation in mutations:
            value = copy.deepcopy(self.translated)
            mutation(value)
            with self.subTest(value=value), self.assertRaises(ValueError):
                validate_translations(value, self.source, "zh-CN")

    def test_chinese_rejects_latin_prose_and_allows_unchanged_proper_names(self):
        value = copy.deepcopy(self.translated)
        value["segments"][0]["text"] = "Hello everyone"
        with self.assertRaisesRegex(ValueError, "Latin prose"):
            validate_translations(value, self.source, "zh-CN")
        for name in ["Alice Smith", "OpenAI", "GPT-4"]:
            with self.subTest(name=name):
                source = [{"id": 1, "start": 0, "end": 1, "text": name}]
                value = {"language": "zh-CN", "segments": [{"id": 1, "text": name}]}
                self.assertEqual(validate_translations(value, source, "zh-CN")[0]["text"], name)

    def test_response_single_json_and_one_enclosing_fence(self):
        with tempfile.TemporaryDirectory() as directory:
            encoded = json.dumps(self.translated, ensure_ascii=False)
            for text in [encoded, "```json\n" + encoded + "\n```", "```\n" + encoded + "\n```"]:
                self.assertEqual(load_translation_response(text, directory), self.translated)
            for text in [encoded + encoded, "commentary " + encoded, "[]", "```json\n{}\n```\n{}",
                         '{"language":"en","language":"zh-CN"}', '{"value":NaN}']:
                with self.subTest(text=text), self.assertRaises(ValueError):
                    load_translation_response(text, directory)

    def test_operator_glossary_preserves_only_names_present_in_source(self):
        source = [{"id": 7, "start": 0, "end": 1000, "text": "Please play silver clouds."}]
        names = ["Silver Clouds", "Alice Smith"]
        for text in ["请播放 Silver Clouds。", "请播放 silver clouds。"]:
            value = {"language": "zh-CN", "segments": [{"id": 7, "text": text}]}
            self.assertEqual(validate_translations(value, source, "zh-CN", names)[0]["text"], text)
        for text in ["请播放银色的云。", "请播放 Silver。", "请播放 Clouds Silver。"]:
            value = {"language": "zh-CN", "segments": [{"id": 7, "text": text}]}
            with self.assertRaisesRegex(ValueError, "Segment 7.*full name.*Silver Clouds"):
                validate_translations(value, source, "zh-CN", names)
        prompt = translation_prompt(source, "zh-CN", names)
        self.assertEqual(json.loads(prompt.split("\n")[-1])["preserve_names"], names)
        self.assertIn("operator glossary", prompt)
        name_source = [{"id": 8, "start": 0, "end": 1000, "text": "silver clouds."}]
        value = {"language": "zh-CN", "segments": [{"id": 8, "text": "silver clouds."}]}
        self.assertEqual(validate_translations(value, name_source, "zh-CN", names)[0]["text"], "silver clouds.")
        source = [{"id": 7, "start": 0, "end": 1000, "text": "I said hello to OpenAI."}]
        value = {"language": "zh-CN", "segments": [{"id": 7, "text": "我向OpenAI问好。"}]}
        self.assertEqual(validate_translations(value, source, "zh-CN", ["AI", "OpenAI"])[0]["text"], "我向OpenAI问好。")
        value["segments"][0]["text"] = "我问好了。"
        validate_translations(value, source, "zh-CN", ["AI"])
        with self.assertRaisesRegex(ValueError, "full name.*OpenAI"):
            validate_translations(value, source, "zh-CN", ["OpenAI"])
        for invalid in ["Silver Clouds", [""], [True]]:
            with self.assertRaises(ValueError):
                validate_translations(value, source, "zh-CN", invalid)

    def test_response_file_is_confined_and_nested_pointers_fail(self):
        with tempfile.TemporaryDirectory() as directory, tempfile.TemporaryDirectory() as outside:
            root = Path(directory)
            path = root / "translated.json"
            path.write_text(json.dumps(self.translated, ensure_ascii=False), encoding="utf-8-sig")
            self.assertEqual(load_translation_response('{"filename":"translated.json"}', root), self.translated)
            for filename in ["../escape.json", "C:\\escape.json", "/escape.json", "sub/../../escape.json", "x.txt", ""]:
                with self.subTest(filename=filename), self.assertRaises(ValueError):
                    load_translation_response(json.dumps({"filename": filename}), root)
            path.write_text('{"filename":"again.json"}', encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "Nested"):
                load_translation_response('{"filename":"translated.json"}', root)
            external = Path(outside) / "outside.json"
            external.write_text("{}", encoding="utf-8")
            try:
                (root / "link.json").symlink_to(external)
            except OSError:
                return  # Windows without symlink privilege still runs every path-boundary case.
            with self.assertRaisesRegex(ValueError, "escapes"):
                load_translation_response('{"filename":"link.json"}', root)

    def test_subtitles_use_long_timestamps_rounding_and_inert_ass_text(self):
        source = [{"id": 0, "start": 90000001, "end": 90000019,
                   "text": "中文{\\b1}\n下一行\\N\x00"}]
        srt, ass = render_srt(source), render_ass(source)
        self.assertIn("25:00:00,001 --> 25:00:00,019", srt)
        self.assertIn("25:00:00.00,25:00:00.02", ass)
        self.assertIn("中文｛＼b1｝\\N下一行＼N ", ass)
        self.assertNotIn("{\\b1}", ass)
        self.assertNotIn("\x00", srt + ass)
        self.assertEqual(srt.count(" --> "), 1)

    def test_srt_text_cannot_create_additional_cues(self):
        source = [{"id": 0, "start": 0, "end": 1000,
                   "text": "\n第一行\n \n2\n00:00:00,001 --> 00:00:00,999\n第二行\n"}]
        srt = render_srt(source)
        self.assertEqual(len(srt.strip().split("\n\n")), 1)
        self.assertTrue(srt.startswith("1\n00:00:00,000 --> 00:00:01,000\n第一行\n"))
        self.assertTrue(srt.endswith("第二行\n\n"))


if __name__ == "__main__":
    unittest.main()
