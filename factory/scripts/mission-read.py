#!/usr/bin/env python3
"""Run an authorized read as argv, recording at most two bounded attempts."""

import json
import subprocess
import sys

ATTEMPT_TIMEOUT = 30
MAX_ATTEMPTS = 2
MAX_REASON = 512


def text_output(value):
    if isinstance(value, bytes):
        return value.decode("utf-8", errors="replace")
    return value or ""


def mission_read(command, required=False, run=subprocess.run):
    records = []
    result = {"available": False, "required": required, "source": subprocess.list2cmdline(command),
              "records": records}
    for _ in range(MAX_ATTEMPTS):
        try:
            completed = run(command, capture_output=True, text=True, encoding="utf-8",
                            errors="replace", timeout=ATTEMPT_TIMEOUT, shell=False)
            record = {"returncode": completed.returncode, "value": text_output(completed.stdout)}
            if completed.returncode in (-2, 130, 0xC000013A):
                records.append(record)
                result.update(attempts=len(records), error="read cancelled", cancelled=True)
                return result, 130
            if completed.returncode:
                record["error"] = f"read exited with status {completed.returncode}"
        except subprocess.TimeoutExpired as error:
            record = {"error": "read timed out after 30 seconds", "value": text_output(error.stdout)}
        except OSError as error:
            record = {"error": f"read could not start: {type(error).__name__}", "value": ""}
        except KeyboardInterrupt:
            records.append({"error": "read cancelled", "value": ""})
            result.update(attempts=len(records), error="read cancelled", cancelled=True)
            return result, 130
        records.append(record)
        result["attempts"] = len(records)
        if record.get("returncode") == 0:
            result.update(available=True, value=record["value"])
            return result, 0
    result["error"] = records[-1]["error"][:MAX_REASON]
    return result, 1 if required else 0


def main(argv=None, run=subprocess.run):
    args = list(sys.argv[1:] if argv is None else argv)
    required = bool(args and args[0] == "--required")
    if required:
        args.pop(0)
    if len(args) < 2 or args[0] != "--" or not args[1].strip():
        print("usage: mission-read.py [--required] -- <read-command> [args...]", file=sys.stderr)
        return 2
    result, code = mission_read(args[1:], required, run)
    print(json.dumps(result, ensure_ascii=False))
    return code


if __name__ == "__main__":
    raise SystemExit(main())
