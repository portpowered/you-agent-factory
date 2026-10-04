"""Copy offline build inputs before executing children in lane-owned paths."""
import hashlib
import os
from pathlib import Path
import re
import shutil
import stat


def owned_paths(root):
    root = Path(root).resolve()
    home = root / "home"
    paths = {key: str(root / key.lower()) for key in (
        "APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME", "XDG_CACHE_HOME",
        "GOCACHE", "GOPATH", "GOMODCACHE", "GOTMPDIR", "TEMP", "TMP")}
    paths.update(HOME=str(home), USERPROFILE=str(home),
                 HOMEDRIVE=home.drive or "/", HOMEPATH=str(home)[len(home.drive):])
    for key, value in paths.items():
        if key not in ("HOMEDRIVE", "HOMEPATH"):
            Path(value).mkdir(parents=True, exist_ok=True)
    return paths


def child_environment(paths):
    # Ignore inherited Go config/workspace/overlay/tool flags as well as network
    # fallbacks. Cached inputs are copied by Python, never by a shared Go child.
    env = dict(os.environ, **paths)
    env.update(GOENV="off", GOWORK="off", GOFLAGS="", GOPROXY="off",
               GOSUMDB="off", GOTOOLCHAIN="auto", GOTELEMETRY="off")
    return env


def escaped_module(module):
    return "".join("!" + c.lower() if c.isupper() else c for c in module)


def seed_inputs(source, output, paths, module_cache=None, go_root=None,
                budget_bytes=2 * 1024**3):
    """Fail closed for unavailable inputs, unsupported modules or disk budget.

    Source go.mod requirements select expanded modules and local metadata;
    no archives, unrelated modules, shared cache writes or downloads.
    """
    text = (source / "go.mod").read_text()
    if re.search(r"^\s*(replace|exclude)\b", text, re.M):
        raise ValueError("offline preparation does not support replace/exclude; supply a supported source")
    version = re.search(r"^go (\S+)$", text, re.M)
    if not version:
        raise ValueError("go.mod must declare an exact cached toolchain version")
    module_cache = Path(module_cache or os.environ.get("GOMODCACHE") or
                        Path(os.environ.get("GOPATH", str(Path.home() / "go"))) / "pkg/mod").resolve()
    if go_root:
        go_root = Path(go_root).resolve()
    else:
        candidates = list((module_cache / "golang.org").glob(
            "toolchain@v0.0.1-go" + version[1] + ".*"))
        platform = "windows" if os.name == "nt" else os.uname().sysname.lower()
        candidates = [p for p in candidates if ("." + platform + "-") in p.name]
        if len(candidates) != 1:
            raise ValueError("supply --cached-go-root for the already cached Go " + version[1])
        go_root = candidates[0]
    suffix = ".exe" if os.name == "nt" else ""
    if not (go_root / "bin" / ("go" + suffix)).is_file():
        raise ValueError("cached Go root missing bin/go: " + str(go_root))
    destination = output / "preparation/toolchain"
    transfers = [(go_root, destination)]
    for module, revision in re.findall(r"^\s*(?:require\s+)?([^\s()]+)\s+(v\S+)", text, re.M):
        escaped = escaped_module(module)
        transfers.append((module_cache / (escaped + "@" + revision),
                          Path(paths["GOMODCACHE"]) / (escaped + "@" + revision)))
        for extension in ("mod", "info", "ziphash"):
            relative = Path("cache/download") / escaped / "@v" / (revision + "." + extension)
            transfers.append((module_cache / relative, Path(paths["GOMODCACHE"]) / relative))
    copies = []
    for origin, target in transfers:
        if not origin.exists():
            raise ValueError("offline cached input missing: " + str(origin) + "; populate outside this lane, then retry")
        if origin.is_dir():
            copies.extend((p, target / p.relative_to(origin)) for p in sorted(origin.rglob("*")) if p.is_file())
        else:
            copies.append((origin, target))
    existing = sum(p.stat().st_size for p in output.rglob("*") if p.is_file())
    additional = sum(p.stat().st_size for p, target in copies if not target.exists())
    # Reserve space for build cache, artifact and temporary compilation files.
    if existing + additional + 512 * 1024**2 > budget_bytes:
        raise ValueError("owned preparation exceeds disk budget including 512MiB build reserve")
    records = []
    for origin, target in copies:
        if origin.is_symlink():
            raise ValueError("cached input symlink unsupported: " + str(origin))
        target.parent.mkdir(parents=True, exist_ok=True)
        # Copy bytes; no hardlinks or junctions to mutable/shared cache state.
        if target.exists():
            target.chmod(target.stat().st_mode | stat.S_IWRITE)
        shutil.copyfile(origin, target)
        shutil.copymode(origin, target)
        records.append(dict(source=str(origin), path=str(target),
                            sha256=hashlib.sha256(target.read_bytes()).hexdigest()))
    paths["GOROOT"] = str(destination)
    env = child_environment(paths)
    env["PATH"] = str(destination / "bin") + os.pathsep + os.environ.get("PATH", "")
    return env, str(destination / "bin" / ("go" + suffix)), records
