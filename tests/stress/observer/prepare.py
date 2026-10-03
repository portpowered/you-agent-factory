"""Prepare an offline, source-preserving Go test overlay for owner observation."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess

PIN = "95e213cfb35b50236fd7a34ad66c797d2ee7b5b6"
HOST = "pkg/services/factory_runtime/internal/services/instance_host/internal/service"
LEASES = "pkg/services/models/internal/services/runtime_host/internal/services/leases/internal/service"
IMPORT = '"github.com/portpowered/infinite-you/pkg/platform/baselineobservation"'


def git(source, *args):
    return subprocess.check_output(["git", "-C", str(source), *args], text=True).strip()


def instrument(text, concrete, hook, template):
    # Fail closed on source drift; only replace the constructor's literal return.
    marker = "return &" + concrete + "{"
    if text.count(marker) != 1:
        raise ValueError("unsupported constructor: " + concrete)
    start = text.index(marker)
    end_marker = "\n\t}, nil" if concrete == "Host" else "\n\t}"
    end = text.index(end_marker, start)
    text = text[:start] + text[start:end].replace(marker, "owner := &" + concrete + "{", 1) + text[end:]
    end = text.index(end_marker, start)
    suffix = "\n\t}\n\t" + hook + "(owner)\n\treturn owner"
    if concrete == "Host":
        suffix += ", nil"
    text = text[:end] + suffix + text[end + len(end_marker):]
    text = text.replace("import (", 'import (\n "sort"\n ' + IMPORT, 1)
    return text + "\n" + template


def prepare(args):
    source = Path(args.source_workspace).resolve()
    output = Path(args.output).resolve()
    tool = Path(__file__).resolve().parent
    if output == source or source in output.parents and not output.is_relative_to(source / ".artifacts"):
        raise ValueError("output must be outside source or under its ignored .artifacts directory")
    head = git(source, "rev-parse", "HEAD")
    if args.mode == "pin" and head != PIN:
        raise ValueError("pin mode requires original pin " + PIN)
    templates = tool / "testdata"
    replacements = {}
    files = []

    def add(virtual, content):
        # Go vet chdirs to virtual package directories even when all files are
        # overlaid. Empty directories add no source bytes or tracked pkg files.
        (source / virtual).parent.mkdir(parents=True, exist_ok=True)
        backing = output / "backing" / virtual
        backing.parent.mkdir(parents=True, exist_ok=True)
        backing.write_text(content, encoding="utf-8", newline="\n")
        subprocess.run(["gofmt", "-w", str(backing)], check=True)
        replacements[str(source / virtual)] = str(backing)
        files.append({"virtual": virtual, "backing": str(backing), "sha256": hashlib.sha256(backing.read_bytes()).hexdigest()})

    add("pkg/platform/baselineobservation/hook.go", (templates / "owner-hook.go.txt").read_text())
    for path, concrete, hook, template in [(HOST, "Host", "observeBaselineHost", "host-snapshot.go.txt"), (LEASES, "service", "observeBaselineLeases", "leases-snapshot.go.txt")]:
        original = (source / path / "service.go").read_text()
        add(path + "/service.go", instrument(original, concrete, hook, (templates / template).read_text()))
    for path, existing, template in [(HOST, "execute_test.go", "host-tests.go.txt"), (LEASES, "service_test.go", "leases-tests.go.txt")]:
        original = (source / path / existing).read_text()
        imports = '\n "fmt"\n ' + IMPORT
        if path == LEASES:
            imports += '\n "sync"'
        add(path + "/" + existing, original.replace("import (", "import (" + imports, 1) + (templates / template).read_text())
    for template, virtual in [("collector-tests.go.txt", "pkg/platform/baselineobservation/hook_test.go"), ("q0-tests.go.txt", "tests/stress/observer/observer_test.go"), ("report.go.txt", "tests/stress/observer/report_test.go"), ("report-tests.go.txt", "tests/stress/observer/report_validation_test.go")]:
        add(virtual, (templates / template).read_text())
    overlay = output / "overlay.json"
    overlay.write_text(json.dumps({"Replace": replacements}, indent=2) + "\n")
    manifest = {"sourceCommit": head, "sourceStatus": git(source, "status", "--porcelain"), "toolSourceCommit": git(tool, "rev-parse", "HEAD"), "mode": args.mode, "files": files, "overlaySHA256": hashlib.sha256(overlay.read_bytes()).hexdigest(), "goVersion": subprocess.check_output(["go", "version"], text=True).strip(), "protocol": "owner-unit-and-Q0; Q1/Q2 not yet implemented", "buildCommand": f'go test -overlay "{overlay}" -p 1 -count=1 -run TestInProcessObserver ./{HOST} ./{LEASES}', "fixture": "controlled component effects; inert BuildProcess Q0; no model or remote calls"}
    (output / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
    print(overlay)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source-workspace", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--mode", choices=["pin", "candidate"], required=True)
    try:
        prepare(parser.parse_args())
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        parser.exit(1, str(error) + "\n")
