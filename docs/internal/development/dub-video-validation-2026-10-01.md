# Dub Video validation — 2026-10-01

## Native Windows factory run

The first complete `@you/dub-video` run exited successfully and published
`C:/Users/andre/work/portos/infinite-you/video-dubbed.mp4`. Its artifacts are in
`C:/Users/andre/work/portos/infinite-you/video-dubbed-dub-90q2cl67`.
The run used the canonical Models operations with `asr`, `llm`, and the
operator-configured `qwen3-tts-base` model, targeting `zh-CN`.

Independent validation used ffprobe, subtitle extraction, PCM decoding, hashes,
and fresh source-reference extraction. No media was played or browser opened.

## Observed results

- MP4: 103,962,512 bytes; duration 48.935000 seconds. Source duration is
  48.934944 seconds. Streams are H.264 video, AAC audio, and `mov_text` subtitles.
- Video packets were copied intact: both source and output produce SHA-256
  `b3a1629e5dbac61de44c3ed651419a4c649424159cb03a037da5e4da7da63bcf`
  when hashed through ffmpeg's video stream-copy output.
- All five source IDs, timestamps, and source texts survive translation and
  synthesis. Every reference WAV hash matches a fresh extraction from its exact
  source interval. The five hashes are distinct and match the saved manifest.
- Each synthesized WAV is nonempty mono PCM at 24 kHz with nonzero samples.
  Fitted PCM contains exactly 24 samples per millisecond for every interval.
- Decoding the published AAC finds nonzero speech in all five intervals. Its
  decoded duration is 48.938667 seconds, including codec padding; audio after
  29 seconds has peak sample value zero.
- UTF-8 SRT and ASS sidecars contain all five Chinese translations. Subtitles
  extracted from the MP4 retain their Unicode text and exact cue boundaries.

| ID | Source interval (ms) | Synthesized seconds | Applied speed | Fitted PCM bytes | Reference SHA-256 prefix |
| --- | --- | --- | --- | --- | --- |
| 0 | 0–5000 | 2.00 | 1.000 | 240000 | `d68b0fc544a3d3da` |
| 1 | 5000–10000 | 5.04 | 1.008 | 240000 | `3563257d70dc041a` |
| 2 | 10000–16000 | 2.88 | 1.000 | 288000 | `a4f281447908efd8` |
| 3 | 16000–21000 | 2.08 | 1.000 | 240000 | `cf133d713c0dd006` |
| 4 | 21000–28000 | 5.92 | 1.000 | 336000 | `dc9da8a715423887` |

Full measured values and hashes are saved outside the repository in
`C:/t/dub-video-validation-20261001/validation.json`.

## Native ASS container proof

A separate validation output at
`C:/t/dub-video-validation-20261001/video-dubbed-ass.mkv` was produced from the
same five fitted PCM files and generated ASS using the media helpers. ffprobe
confirms H.264, AAC, and native ASS streams. Duration is 48.978000 seconds,
including AAC/container padding. The original MP4 and factory manifest were
not changed by this check.

The first MP4 lacks audio/subtitle language tags because its mux command passed
BCP 47 `zh-CN`; MP4 expects an ISO 639 language code. The MKV retains `zh-CN`.
This metadata issue was reported for a container-specific correction.

## Source integrity and quality limits

`demo.mp4` remains 105,306,116 bytes, matching the recorded pre-run size.
No pre-run whole-file digest was captured, so unchanged file identity cannot be
claimed from a before/after hash comparison. Its post-run SHA-256 is
`603d3a7469ab066fccf96e609756d7bf1a2707c658e3360f6ad61a3c9a3ceef1`.

The initial translation renders “bubble tea and cigarettes” literally as
“珍珠奶茶和香烟”. In a music request this may identify a band or title and should
be preserved as a name rather than translated literally. The translation prompt
is being corrected before the final rerun; this report describes the initial run.

Native Qwen runtime evidence confirms original reference samples were encoded
into speaker embeddings and reference tokens with Chinese language conditioning.
Pronunciation accuracy, perceived voice similarity, and speaker separation were
not independently assessed by listening. Distinct reference hashes establish
different source clips, not a perceptual similarity score.

The rendered audio replaces the original audio track. This run performs no
dialogue/music separation and contains silence after the last dubbed line;
preserving original background music would require additional processing.

## Subsequent corrections and semantic audit limits

The second complete run, `video-dubbed-dub-8zupi1u9`, fixes MP4 language metadata
by using the container language code `zho` while retaining customer language
`zh-CN`. ffprobe confirms the Chinese audio language tag. Its translation still
contains two substantive errors:

- Segment 0 becomes “我现在能帮您忙吗？” (“Can I help you now?”), reversing the
  source speaker/addressee relationship in “Can you assist me right now?”
- Segment 2 still translates the music name literally instead of preserving it.

The third complete run, `video-dubbed-dub-a6lcianw`, corrects segment 0 but
renders segment 2 as “你能播放奶茶和香烟吗？”. Its saved
`translation-audit-attempt-1.txt` returns `valid: true` and an empty issues list.
This is direct evidence that a semantic audit by the same local LLM can approve
an incorrect proper-name translation. A successful audit is not an independent
translation-quality guarantee.

“Bubble Tea and Cigarettes” is a band name, confirmed by the band's
[official biography](https://bubbleteaandcigarettes.com/bio). The final run uses
the operator glossary `--preserve-names "Bubble Tea and Cigarettes"`, with a
deterministic check that the protected name survives translation. This confines
the known-name failure; names omitted from the glossary and other semantic
errors still depend on model quality and review.

## Final glossary-protected run

The final run, `video-dubbed-dub-aafoean5`, completed successfully with
`--preserve-names "Bubble Tea and Cigarettes"`. Its saved semantic audit returns
`valid: true`; the independent deterministic check also confirms the exact
protected name in translated segment 2, the SRT/ASS sidecars, extracted MP4
subtitles, and extracted native MKV ASS. Segment 0 is now “您现在能帮我吗？”,
which preserves the source speaker/addressee relationship.

Final independent checks pass:

- Published MP4: 103,944,990 bytes; duration 48.935000 seconds; H.264 video,
  AAC audio, and `mov_text` subtitles. Both audio and subtitle language tags are
  `zho`. All five subtitle intervals remain exactly 0–5, 5–10, 10–16, 16–21,
  and 21–28 seconds.
- All five reference hashes match fresh source extraction and the hashes in the
  initial-run table. All fitted PCM byte counts and source IDs/text/timestamps
  remain exact. Every synthesized WAV and every published AAC interval contains
  nonzero samples. No source video packets changed.
- Source remains 105,306,116 bytes with whole-file SHA-256
  `603d3a7469ab066fccf96e609756d7bf1a2707c658e3360f6ad61a3c9a3ceef1`,
  matching the digest captured after the initial run. This establishes stability
  between those observations, not a pre-initial-run hash comparison.
- The independently derived final MKV at
  `C:/t/dub-video-final-validation-20261001/video-dubbed-ass.mkv` contains
  H.264/AAC/native ASS; all five extracted Unicode cues and the protected name
  survive. Its 48.978-second duration includes AAC/container padding.
- Raw final measurements are saved in
  `C:/t/dub-video-final-validation-20261001/validation.json`. Helper imports used
  `python -B` and `PYTHONDONTWRITEBYTECODE=1`; no Python cache files were created
  inside the packaged factory.

| ID | Final text | Synthesized seconds | Applied speed |
| --- | --- | --- | --- |
| 0 | 您现在能帮我吗？ | 1.20 | 1.000 |
| 1 | 当然可以，我的人类，我能帮您做什么？ | 8.64 | 1.728 |
| 2 | 你能播放 Bubble Tea and Cigarettes 吗？ | 5.44 | 1.000 |
| 3 | 正在正常运行，先生。 | 2.00 | 1.000 |
| 4 | 你能恢复音乐吗？ | 3.84 | 1.000 |

Segment 1 needed 1.728× time compression to fit its five-second source interval.
The pipeline preserves the full fitted interval, but intelligibility at that
speed was not assessed by listening. Pronunciation and perceived voice similarity
also remain unassessed. The default installed ASR model is English-only
`whisper-base.en`; it was not used to make a misleading Chinese pronunciation
claim. No media was played or browser opened during final validation.
