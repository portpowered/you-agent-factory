"""Prepare an offline, source-preserving Go test overlay for owner observation."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
from profile_prepare import build_profile
from preparation_environment import owned_paths, child_environment, seed_inputs

PIN = "95e213cfb35b50236fd7a34ad66c797d2ee7b5b6"
HOST = "pkg/services/factory_runtime/internal/services/instance_host/internal/service"
LEASES = "pkg/services/models/internal/services/runtime_host/internal/services/leases/internal/service"
IMPORT = '"github.com/portpowered/infinite-you/pkg/platform/baselineobservation"'


def git(source, *args, env=None):
    return subprocess.check_output(["git", "-C", str(source), *args], text=True, env=env).strip()


def instrument(text, concrete, hook, template):
    # Scope views can share an owner's registry. Instrument only New so the
    # same registry is not registered twice through those compatibility views.
    constructor_start = text.index("func New(")
    constructor_end = text.index("\n}", constructor_start)
    constructor = text[constructor_start:constructor_end]
    marker = "return &" + concrete + "{"
    if constructor.count(marker) != 1:
        raise ValueError("unsupported constructor: " + concrete)
    end_marker = "}, nil" if concrete == "Host" else "}"
    if not constructor.endswith(end_marker):
        raise ValueError("unsupported constructor return: " + concrete)
    constructor = constructor.replace(marker, "owner := &" + concrete + "{", 1)
    suffix = "}\n\t" + hook + "(owner)\n\treturn owner"
    if concrete == "Host":
        suffix += ", nil"
    constructor = constructor[:-len(end_marker)] + suffix
    text = text[:constructor_start] + constructor + text[constructor_end:]
    imports = '\n "sort"\n ' + IMPORT
    if '"fmt"' not in text:
        imports += '\n "fmt"'
    text = text.replace("import (", "import (" + imports, 1)
    return text + "\n" + template


def prepare(args):
    source = Path(args.source_workspace).resolve()
    output = Path(args.output).resolve()
    tool = Path(__file__).resolve().parent
    if output == source or source in output.parents and not output.is_relative_to(source / ".artifacts"):
        raise ValueError("output must be outside source or under its ignored .artifacts directory")
    if args.build_profile:
        (output / "profile-manifest.json").unlink(missing_ok=True)
    preparation_paths = owned_paths(output / "preparation/environment") if args.build_profile else None
    env = child_environment(preparation_paths) if preparation_paths else None
    go_command, gofmt_command, seed_records = "go", "gofmt", []
    head = git(source, "rev-parse", "HEAD", env=env)
    if args.mode == "pin" and head != PIN:
        raise ValueError("pin mode requires original pin " + PIN)
    templates = tool / "testdata"
    if args.build_profile:
        # Never leave an old qualified handoff after a failed new preparation.
        if git(source, "status", "--porcelain", env=env) or git(tool, "status", "--porcelain", env=env):
            raise ValueError("--build-profile requires clean source and committed clean tooling")
        env, go_command, seed_records = seed_inputs(source, output, preparation_paths,
            getattr(args, "cached_module_cache", None), getattr(args, "cached_go_root", None))
        gofmt_command = str(Path(go_command).with_name("gofmt" + Path(go_command).suffix))
    replacements = {}
    files = []

    def add(virtual, content):
        # Go vet chdirs to virtual package directories even when all files are
        # overlaid. Empty directories add no source bytes or tracked pkg files.
        (source / virtual).parent.mkdir(parents=True, exist_ok=True)
        backing = output / "backing" / virtual
        backing.parent.mkdir(parents=True, exist_ok=True)
        backing.write_text(content, encoding="utf-8", newline="\n")
        subprocess.run([gofmt_command, "-w", str(backing)], check=True, env=env)
        replacements[str(source / virtual)] = str(backing)
        files.append({"virtual": virtual, "backing": str(backing), "sha256": hashlib.sha256(backing.read_bytes()).hexdigest()})

    add("pkg/platform/baselineobservation/hook.go", (templates / "owner-hook.go.txt").read_text())
    for path, concrete, hook, template in [(HOST, "Host", "observeBaselineHost", "host-snapshot.go.txt"), (LEASES, "service", "observeBaselineLeases", "leases-snapshot.go.txt")]:
        original = (source / path / "service.go").read_text()
        add(path + "/service.go", instrument(original, concrete, hook, (templates / template).read_text()))
    for path, existing, template in [(HOST, "execute_test.go", "host-tests.go.txt"), (LEASES, "service_test.go", "leases-tests.go.txt")]:
        original = (source / path / existing).read_text()
        imports = '\n "fmt"\n ' + IMPORT
        calibration = (templates / template).read_text()
        if path == HOST and '"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host"' not in original:
            imports += '\n instancehost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host"'
        if path == HOST and head == PIN:
            # Only the original pin lacks the testing.T cleanup argument.
            # Current candidates retain their helper's fixture cleanup.
            calibration = calibration.replace(
                "newLifecycleControlFactory(t, ", "newLifecycleControlFactory("
            )
        if path == LEASES:
            imports += '\n "sync"'
            if head == PIN:
                # The original owner binds its coordinator after construction.
                # Adapt only calibration setup; production overlay still just reads.
                calibration = calibration.replace(
                    "internalservice.New(clock, readySlotFacts{capacity: 2}, coordinator)",
                    "internalservice.New(clock, readySlotFacts{capacity: 2})\n\tleaseswire.BindCoordinator(owner, coordinator)",
                ).replace(
                    "internalservice.New(fixedHostClock{}, readySlotFacts{capacity: 2}, noopCoordinator{})",
                    "internalservice.New(fixedHostClock{}, readySlotFacts{capacity: 2})\n\tleaseswire.BindCoordinator(owner, &recordingSlotCapacityCoordinator{})",
                )
        add(path + "/" + existing, original.replace("import (", "import (" + imports, 1) + calibration)
    for template, virtual in [("collector-tests.go.txt", "pkg/platform/baselineobservation/hook_test.go"), ("q0-tests.go.txt", "tests/stress/observer/observer_test.go"), ("lifecycle-tests.go.txt", "tests/stress/observer/lifecycle_test.go"), ("report.go.txt", "tests/stress/observer/report_test.go"), ("report-tests.go.txt", "tests/stress/observer/report_validation_test.go"), ("profile-tests.go.txt", "tests/stress/observer/profile_test.go"), ("profile-report.go.txt", "tests/stress/observer/profile_report_test.go"), ("profile-cycles.go.txt", "tests/stress/observer/profile_cycles_test.go"), ("profile-model-gate.go.txt", "tests/stress/observer/profile_model_gate_test.go")]:
        add(virtual, (templates / template).read_text())
    overlay = output / "overlay.json"
    overlay.write_text(json.dumps({"Replace": replacements}, indent=2) + "\n")
    manifest = {"sourceCommit": head, "sourceStatus": git(source, "status", "--porcelain", env=env), "toolSourceCommit": git(tool, "rev-parse", "HEAD", env=env), "mode": args.mode, "files": files, "overlaySHA256": hashlib.sha256(overlay.read_bytes()).hexdigest(), "goVersion": subprocess.check_output([go_command, "version"], text=True, env=env).strip(), "protocol": "owner-unit-and-Q0/Q1/Q2; terminal-session replacement", "buildCommand": f'go test -c -overlay "{overlay}" -p 1 -o "{output / "observer.test.exe"}" ./tests/stress/observer', "fixture": "one root process; controlled Codex command edge; idle bootstrap; explicit session Work/terminal replacement/close; no model or remote calls"}
    (output / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
    if args.build_profile:
        build_profile(source, output, tool, manifest, files, args.mode,
                      env, go_command, preparation_paths, seed_records)
    print(overlay)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source-workspace", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--mode", choices=["pin", "candidate"], required=True)
    parser.add_argument("--build-profile", action="store_true", help="compile and attest a prebuilt profile artifact; requires clean committed tooling")
    parser.add_argument("--cached-module-cache", help="read-only expanded module input cache (defaults to inherited GOMODCACHE/GOPATH)")
    parser.add_argument("--cached-go-root", help="read-only already cached Go root (defaults to exact go.mod cached auto toolchain)")
    try:
        prepare(parser.parse_args())
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        parser.exit(1, str(error) + "\n")
