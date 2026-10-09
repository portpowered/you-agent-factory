#!/usr/bin/env python3
"""Project valid mission replies; request exactly one shape correction."""

import json
import math
import sys

INVALID_PREFIX = "mission-output-invalid:"
MAX_DIAGNOSTIC = 512


def nonblank(value):
    return isinstance(value, str) and bool(value.strip())


def reject_constant(_):
    raise ValueError("non-JSON number")


def finite_float(raw):
    value = float(raw)
    if not math.isfinite(value):
        raise ValueError("number exceeds finite range")
    return value


def validate_reply(reply):
    if not isinstance(reply, dict):
        return "reply must be a JSON object"
    if reply.get("decision") not in ("ACCEPTED", "FAILED", "PRECONDITION"):
        return "decision must be ACCEPTED, FAILED or PRECONDITION"
    if not isinstance(reply.get("feedback"), str):
        return "feedback must be a string"
    output = reply.get("output")
    if not isinstance(output, dict):
        return "output must be a native object with measurements or a named unmet precondition"
    if reply["decision"] == "PRECONDITION" and "precondition" not in output:
        return "PRECONDITION requires output.precondition"
    if "precondition" in output:
        precondition = output["precondition"]
        if not (nonblank(precondition) or
                isinstance(precondition, dict) and any(nonblank(value) for value in precondition.values())):
            return "output.precondition must name the unmet precondition"
    if "measurements" in output:
        measurements = output["measurements"]
        if not isinstance(measurements, list) or not measurements:
            return "output.measurements must be a non-empty list"
        for index, measurement in enumerate(measurements):
            if not isinstance(measurement, dict):
                return f"output.measurements[{index}] must be an object"
            for field in ("name", "source"):
                if not nonblank(measurement.get(field)):
                    return f"output.measurements[{index}].{field} must be non-blank"
            if "value" not in measurement:
                return f"output.measurements[{index}].value is required"
    elif "precondition" not in output:
        return "output requires measurements or a named unmet precondition"
    return None


def check_mission_output(raw, rejection_feedback=""):
    try:
        reply = json.loads(raw, parse_constant=reject_constant, parse_float=finite_float)
        reason = validate_reply(reply)
        if not reason:
            reply = {field: reply[field] for field in ("decision", "feedback", "output")}
            # Keep serialization failures inside the same bounded correction path.
            json.dumps(reply, ensure_ascii=False, allow_nan=False)
    except (ValueError, TypeError, OverflowError, RecursionError):
        reason = "reply must be valid JSON"
    if reason:
        return {
            "decision": "FAILED" if INVALID_PREFIX in rejection_feedback else "REJECTED",
            "feedback": INVALID_PREFIX + " " + reason,
            "output": {"invalidReply": raw[:MAX_DIAGNOSTIC]},
        }
    # PRECONDITION belongs only to the verifier. The factory consumes its
    # existing CONTINUE envelope, retaining all available evidence unchanged.
    output = reply["output"]
    if reply["decision"] == "PRECONDITION" or (
            "precondition" in output and "invalidReply" not in output and (
                reply["decision"] == "ACCEPTED" or "measurements" not in output)):
        reply["decision"] = "CONTINUE"
    return reply


def main(argv=None):
    args = sys.argv[1:] if argv is None else argv
    if len(args) != 2:
        print("usage: check-mission-output.py <raw-reply> <rejection-feedback>", file=sys.stderr)
        return 2
    print(json.dumps(check_mission_output(*args), ensure_ascii=False, allow_nan=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
