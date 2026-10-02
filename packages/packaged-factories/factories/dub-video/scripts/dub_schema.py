"""JSON Schema response contracts for text-only Models generation.

A schema constrains representation; source meaning still requires validation and
an independent semantic audit. Text fields stay unbounded so Unicode, embedded
quotes, and legal JSON escapes remain generatable, and no thinking, token, or
word-count cap is expressed here.

The pinned native converter reads ``prefixItems`` only when ``items`` is absent,
so the ordered cue tuple carries ``prefixItems`` with exact ``minItems`` and
``maxItems`` and never an ``items`` keyword.
"""

from __future__ import annotations


def _text():
    """Any nonempty JSON string, without a length cap or character pattern."""
    return {"type": "string", "minLength": 1}


def translation_schema(segments, language):
    """One closed object: the requested language plus one ordered cue per segment."""
    cues = [{"type": "object",
             "properties": {"id": {"const": segment["id"]}, "text": _text()},
             "required": ["id", "text"], "additionalProperties": False}
            for segment in segments]
    return {"type": "object",
            "properties": {
                "language": {"const": language},
                "segments": {"type": "array", "prefixItems": cues,
                             "minItems": len(cues), "maxItems": len(cues)}},
            "required": ["language", "segments"], "additionalProperties": False}


def audit_schema(identifiers):
    """One closed decision: no issues, or at least one indexed issue object.

    Segment IDs are constrained to the supplied set, so duplicate or unknown IDs
    remain a post-generation validation failure rather than a schema concern.
    """
    issue = {"type": "object",
             "properties": {"segment_id": {"enum": sorted(identifiers)},
                            "suggested_correction": _text()},
             "required": ["segment_id", "suggested_correction"],
             "additionalProperties": False}
    return {"type": "object",
            "oneOf": [
                {"type": "object",
                 "properties": {"valid": {"const": True}, "issues": {"const": []}},
                 "required": ["valid", "issues"], "additionalProperties": False},
                {"type": "object",
                 "properties": {"valid": {"const": False},
                                "issues": {"type": "array", "minItems": 1, "items": issue}},
                 "required": ["valid", "issues"], "additionalProperties": False},
            ]}
