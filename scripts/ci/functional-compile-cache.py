"""Retain bounded Go compiler artifacts without caching test executions.

Go marks cache entries used at most once per hour. The one-hour margin retains
objects read during this job, including hits restored with an earlier mtime.
Archives, static coverage metadata and action metadata preserve Go's own content-based invalidation;
executables, test results, generated-file directories and unrelated data stay
out of the transferable cache.
"""
import argparse
from datetime import datetime, timezone
from pathlib import Path
import shutil
import time

ARCHIVE_MAGIC = b"!<arch>\n"
COVERAGE_META_MAGIC = b"\x00cvm"


def entries(root):
    for path in root.glob("*/*"):
        if path.is_symlink() or not path.is_file():
            continue
        name = path.name
        if len(name) != 66 or name[-2:] not in ("-a", "-d"):
            continue
        if any(char not in "0123456789abcdef" for char in name[:64]):
            continue
        if path.parent.name != name[:2]:
            continue
        yield path


def compiler_entry(path):
    with path.open("rb") as source:
        prefix = source.read(8)
    return prefix.startswith(b"v1 ") if path.name.endswith("-a") else prefix == ARCHIVE_MAGIC or prefix.startswith(COVERAGE_META_MAGIC)


def transfer(source, destination, cutoff=None, limit=None):
    destination.mkdir(parents=True, exist_ok=True)
    candidates = []
    for path in entries(source):
        info = path.stat()
        if cutoff is not None and info.st_mtime < cutoff:
            continue
        if compiler_entry(path):
            candidates.append((path, info))
    # Metadata is small and retained first; recent archives take precedence
    # when the byte budget is reached. Missing outputs are ordinary Go misses.
    candidates.sort(key=lambda item: (not item[0].name.endswith("-a"), -item[1].st_mtime))
    copied, total, omitted = 0, 0, 0
    for path, info in candidates:
        if limit is not None and total + info.st_size > limit:
            omitted += 1
            continue
        target = destination / path.relative_to(source)
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(path, target)
        copied += 1
        total += info.st_size
    return {"files": copied, "bytes": total, "omitted": omitted}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mode", choices=("capture", "restore"))
    parser.add_argument("go_cache", type=Path)
    parser.add_argument("snapshot", type=Path)
    parser.add_argument("--job-start", default="")
    parser.add_argument("--max-bytes", type=int, default=1024**3)
    args = parser.parse_args()
    if args.go_cache.resolve() == args.snapshot.resolve():
        parser.error("snapshot and Go cache must be separate directories")
    if args.max_bytes <= 0:
        parser.error("max-bytes must be positive")
    if args.mode == "restore":
        result = transfer(args.snapshot, args.go_cache, limit=args.max_bytes)
    else:
        # A fresh capture directory avoids retaining evicted outputs from a
        # restored snapshot. The invoking workflow supplies a new destination.
        if args.snapshot.exists() and any(args.snapshot.iterdir()):
            parser.error("capture destination must be empty")
        started = time.time()
        if args.job_start:
            parsed = datetime.fromisoformat(args.job_start.replace("Z", "+00:00"))
            started = parsed.replace(tzinfo=timezone.utc).timestamp() if parsed.tzinfo is None else parsed.timestamp()
        result = transfer(args.go_cache, args.snapshot, started - 3600, args.max_bytes)
    print(f"Functional compiler cache: mode={args.mode} files={result['files']} bytes={result['bytes']} omitted={result['omitted']}")


if __name__ == "__main__":
    main()
