#!/usr/bin/env python3
"""Bounded retries for ideafy's session and explicit-session Work inspection.

Legacy CLI forms retry only recognized HTTP 5xx and transport timeouts.
Optional ALL inventory retries any failed read once with a longer timeout.
subprocess.run kills and reaps a timed-out child. No shell or writes.
"""

import json
from http.client import HTTPException
import random
import re
import subprocess
import sys
import time
from urllib.parse import urlsplit
from urllib.request import Request, urlopen
from uuid import UUID


BACKOFF = (1, 2, 4)
ATTEMPT_TIMEOUT = 30
INVENTORY_TIMEOUTS = (10, 60)
MAX_JITTER = 0.25
DIAGNOSTIC_PREFIX = r"^(?:Error: |debug: cause\[\d+\]=)?"
STATUS = re.compile(
    DIAGNOSTIC_PREFIX + r"(?:list factory sessions|list work|work list page [1-9]\d*) failed \((\d{3})\)",
    re.MULTILINE
)
TRANSPORT_TIMEOUT = re.compile(
    DIAGNOSTIC_PREFIX + r'(?:factory (?:sessions endpoint )?not reachable at [^\r\n]+: )?'
    r'Get "[^"\r\n]+":[^\r\n]*'
    r'(?:context deadline exceeded|Client\.Timeout exceeded|i/o timeout)', re.MULTILINE
)


def command_for(args):
    """Allow only the prompt's read forms, with explicit authority."""
    if len(args) not in (4, 6) or args[0] != "--server":
        raise ValueError("expected --server <url> session list/inventory or work list --session <UUID>")
    server = urlsplit(args[1])
    if (server.scheme not in ("http", "https") or not server.hostname
            or server.username or server.password or server.query or server.fragment):
        raise ValueError("server must be an HTTP(S) URL without credentials, query or fragment")
    # Accessing port also rejects malformed port strings before starting a child.
    server.port
    if args[2:] == ["session", "list"]:
        return ["you", "--debug", *args], "session-list"
    if args[2:] == ["session", "inventory"]:
        return args[1].rstrip("/") + "/factory-sessions?scope=all", "session-inventory"
    if len(args) == 6 and args[2:5] == ["work", "list", "--session"]:
        UUID(args[5])
        return ["you", "--debug", *args], "work-list"
    raise ValueError("only session list/inventory and work list --session <UUID> are permitted")


def inventory_read(url, *, http, sleep, jitter, stdout, stderr):
    """Optional read: every admitted failure gets one longer retry; no bodies in diagnostics."""
    for attempt, timeout in enumerate(INVENTORY_TIMEOUTS, 1):
        try:
            with http(Request(url, method="GET"), timeout=timeout) as response:
                if response.status != 200:
                    raise ValueError("unsuccessful inventory status")
                body = response.read()
            payload = json.loads(body)
            if not isinstance(payload, dict) or not isinstance(payload.get("sessions"), list):
                raise ValueError("invalid inventory response")
        except (OSError, ValueError, UnicodeError, HTTPException):
            stderr.write((f"ideafy-read: session-inventory attempt={attempt}/2"
                          f" class=transient timeout={timeout}s\n").encode("utf-8"))
            if attempt == len(INVENTORY_TIMEOUTS):
                stderr.write(b"ideafy-read: optional session inventory gap after 2 attempts; perform the bound mission\n")
                return 1
            sleep(1 + max(0, min(MAX_JITTER, jitter())))
            continue
        stdout.write(body)
        return 0


def retry_class(stderr):
    status = STATUS.search(stderr)
    if status:
        return "http-5xx" if 500 <= int(status[1]) <= 599 else None
    if TRANSPORT_TIMEOUT.search(stderr):
        return "transport-timeout"
    return None


def main(args, *, runner=subprocess.run, sleep=time.sleep,
         jitter=lambda: random.uniform(0, MAX_JITTER), http=urlopen, stdout=None, stderr=None):
    stdout = sys.stdout.buffer if stdout is None else stdout
    stderr = sys.stderr.buffer if stderr is None else stderr
    try:
        command, family = command_for(args)
    except ValueError as error:
        stderr.write(f"ideafy-read: invalid arguments: {error}\n".encode("utf-8"))
        return 2

    if family == "session-inventory":
        return inventory_read(command, http=http, sleep=sleep, jitter=jitter,
                              stdout=stdout, stderr=stderr)

    for attempt in range(1, len(BACKOFF) + 2):
        try:
            result = runner(command, capture_output=True, timeout=ATTEMPT_TIMEOUT)
            result.stdout.decode("utf-8", errors="strict")
            diagnostic = result.stderr.decode("utf-8", errors="strict")
            if result.returncode == 0:
                stdout.write(result.stdout)
                stderr.write(result.stderr)
                return 0
            code = result.returncode
            category = retry_class(diagnostic)
        except subprocess.TimeoutExpired:
            code, category = 1, "subprocess-timeout"
            diagnostic = f"ideafy-read: {family} exceeded {ATTEMPT_TIMEOUT}s attempt timeout\n"
        except (OSError, UnicodeError) as error:
            code, category = 1, None
            diagnostic = f"ideafy-read: {family} local execution failure: {error}\n"

        if category is None or attempt > len(BACKOFF):
            stderr.write(diagnostic.encode("utf-8"))
            if diagnostic and not diagnostic.endswith("\n"):
                stderr.write(b"\n")
            stderr.write((f"ideafy-read: {family} failed after {attempt} attempt(s)"
                          f"; class={category or 'permanent-or-local'}\n").encode("utf-8"))
            return code
        delay = BACKOFF[attempt - 1] + max(0, min(MAX_JITTER, jitter()))
        stderr.write((f"ideafy-read: {family} attempt={attempt}/4 class={category}"
                      f" retry-delay={delay:.3f}s\n").encode("utf-8"))
        sleep(delay)


if __name__ == "__main__":
    try:
        sys.exit(main(sys.argv[1:]))
    except KeyboardInterrupt:
        sys.exit(130)
