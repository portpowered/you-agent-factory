"""GBNF response contracts for text-only Models generation.

Grammar constrains representation; source meaning still requires validation and
an independent semantic audit. JSON strings retain Unicode and legal escapes.
"""

import json


# Unbounded inter-token whitespace allowed native generation to spend hundreds
# of tokens on blank lines instead of advancing to the next required cue.
# One optional character keeps spacing flexible without an unlimited stall.
# This does not constrain whitespace or length inside translated JSON strings.
JSON_RULES = r'''
ws ::= [ \t\n\r]?
text ::= "\"" character+ "\"" ws
character ::= [^"\\\x00-\x1F] | "\\" (["\\/bfnrt] | "u" [0-9a-fA-F]{4})
'''


def literal(value):
    """Quote a complete JSON value as a GBNF terminal."""
    return json.dumps(json.dumps(value, ensure_ascii=False), ensure_ascii=False)


def translation_grammar(segments, language):
    cues = []
    for segment in segments:
        cues.append('"{" ws ' + literal("id") + ' ws ":" ws '
                    + literal(segment["id"]) + ' ws "," ws '
                    + literal("text") + ' ws ":" ws text "}" ws')
    body = ' "," ws '.join(cues)
    return ('root ::= ws "{" ws ' + literal("language") + ' ws ":" ws '
            + literal(language) + ' ws "," ws ' + literal("segments")
            + ' ws ":" ws "[" ws ' + body + ' "]" ws "}" ws\n' + JSON_RULES)


def audit_grammar(identifiers):
    ids = " | ".join(literal(identifier) for identifier in sorted(identifiers))
    return ('root ::= ws "{" ws ' + literal("valid") + ' ws ":" ws result "}" ws\n'
            'result ::= "true" ws "," ws ' + literal("issues") + ' ws ":" ws "[" ws "]" ws'
            ' | "false" ws "," ws ' + literal("issues")
            + ' ws ":" ws "[" ws issue ("," ws issue)* "]" ws\n'
            'issue ::= "{" ws ' + literal("segment_id") + ' ws ":" ws identifier ws "," ws '
            + literal("suggested_correction") + ' ws ":" ws text "}" ws\n'
            'identifier ::= ' + ids + '\n' + JSON_RULES)
