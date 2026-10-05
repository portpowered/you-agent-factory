"""Build the supported custom binary and retain its actual host for deadcode."""

import argparse
import os
from pathlib import Path
import subprocess


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--destination", required=True)
    parser.add_argument("command", nargs=argparse.REMAINDER)
    args = parser.parse_args()
    command = args.command
    if command and command[0] == "--":
        command = command[1:]
    if not command:
        parser.error("a golangci custom build command is required")

    destination = Path(args.destination)
    destination.mkdir(parents=True, exist_ok=True)
    pointer = destination / "host-path.txt"
    pointer.unlink(missing_ok=True)
    environment = dict(os.environ, CUSTOM_GCL_KEEP_TEMP_FILES="1")
    marker = "the temporary directory is preserved: "
    host = None
    with subprocess.Popen(
        command, env=environment, stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT, text=True,
    ) as process:
        for line in process.stdout:
            print(line, end="", flush=True)
            if marker in line:
                host = Path(line.split(marker, 1)[1].strip()) / "golangci-lint"
        status = process.wait()
    if status:
        return status
    if host is None or not (host / "go.mod").is_file():
        raise RuntimeError("custom build did not retain its generated golangci host")
    pointer.write_text(str(host.resolve()) + "\n", encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
