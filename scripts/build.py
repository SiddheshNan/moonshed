#!/usr/bin/env python3
"""Build Moonshed release bundles.

Optionally vets and tests the Go code, builds the UI, cross-compiles the binary
and assembles one bundle per target:

    dist/
      linux-amd64/
        moonshed
        runtime/
          config/production.yaml
          web/

Copy a bundle to the server and run:  ./moonshed install -e production
"""

import os
import platform
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
UI = ROOT / "ui"
RUNTIME = ROOT / "runtime"
PRODUCTION_CONFIG = RUNTIME / "config" / "production.yaml"
VERSION_GO = ROOT / "internal" / "version" / "version.go"
PACKAGE_JSON = UI / "package.json"
VERSION_RE = re.compile(r'const Version = "([^"]*)"')
PACKAGE_VERSION_RE = re.compile(r'("version":\s*)"[^"]*"')
SEMVER_RE = re.compile(r"\d+\.\d+\.\d+")
RELEASE_TARGETS = ["linux/amd64", "linux/arm64", "windows/amd64"]


class BuildError(Exception):
    pass


def step(msg: str) -> None:
    print(f"==> {msg}", flush=True)


def tool(name: str) -> str:
    path = shutil.which(name)
    if not path:
        raise BuildError(f"'{name}' was not found on PATH")
    return path


def command_line(cmd: list) -> str:
    return " ".join([Path(cmd[0]).name, *cmd[1:]])


def run(cmd: list, cwd: Path, env: dict = None) -> None:
    """Run a build step, streaming its output to the terminal."""
    print(f"    $ {command_line(cmd)}", flush=True)
    result = subprocess.run(cmd, cwd=cwd, env={**os.environ, **(env or {})})
    if result.returncode != 0:
        raise BuildError(f"{command_line(cmd)} failed (exit code {result.returncode})")


def native_target() -> str:
    arch = {"x86_64": "amd64", "amd64": "amd64", "arm64": "arm64", "aarch64": "arm64"}.get(platform.machine().lower(), platform.machine())
    return f"{platform.system().lower()}/{arch}"


def current_version() -> str:
    match = VERSION_RE.search(VERSION_GO.read_text())
    if not match:
        raise BuildError(f"no version found in {VERSION_GO}")
    return match.group(1)


def set_version(ver: str) -> None:
    """Write the release version to the Go version package and the UI's package.json."""
    VERSION_GO.write_text(VERSION_RE.sub(f'const Version = "{ver}"', VERSION_GO.read_text()))
    PACKAGE_JSON.write_text(PACKAGE_VERSION_RE.sub(rf'\g<1>"{ver}"', PACKAGE_JSON.read_text(), count=1))


def test_go(go: str) -> None:
    step("Vetting Go code")
    run([go, "vet", "./..."], ROOT)
    run([go, "vet", "./..."], ROOT, env={"GOOS": "windows"})

    step("Running tests")
    run([go, "test", "./..."], ROOT)


def build_ui() -> Path:
    npm = tool("npm")
    step("Installing UI dependencies (npm ci)")
    run([npm, "ci"], UI)
    step("Building UI (npm run build)")
    run([npm, "run", "build"], UI)

    dist = UI / "dist"
    if not (dist / "index.html").is_file():
        raise BuildError(f"UI build did not produce {dist / 'index.html'}")
    return dist


def build_binary(go: str, target: str, out_dir: Path, ver: str) -> Path:
    goos, _, goarch = target.partition("/")
    binary = out_dir / goos / goarch / ("moonshed.exe" if goos == "windows" else "moonshed")
    step(f"Building moonshed v{ver} for {target}")
    run(
        [go, "build", "-trimpath", "-ldflags", "-s -w", "-o", str(binary), "./cmd/moonshed"],
        ROOT,
        env={"CGO_ENABLED": "0", "GOOS": goos, "GOARCH": goarch},
    )
    return binary


def check_output_dir(out: Path) -> None:
    """Refuse to replace anything that is not empty or a previous build, since assemble deletes it."""
    protected = {Path.home().resolve(), ROOT, *ROOT.parents}
    if out in protected or out == RUNTIME or RUNTIME in out.parents:
        raise BuildError(f"refusing to use {out} as the output directory")
    if out.exists() and any(out.iterdir()):
        is_build = all((bundle / "runtime").is_dir() for bundle in out.iterdir() if bundle.is_dir())
        if not is_build:
            raise BuildError(f"{out} is not empty and is not a previous build; choose another output directory")


def assemble(out: Path, binaries: dict, ui_dist: Path) -> None:
    step(f"Assembling {out}")
    staging = out.with_name(out.name + ".tmp")
    shutil.rmtree(staging, ignore_errors=True)

    for target, binary in binaries.items():
        bundle = staging / target.replace("/", "-")
        (bundle / "runtime" / "config").mkdir(parents=True)
        shutil.copy2(binary, bundle / binary.name)
        shutil.copy2(PRODUCTION_CONFIG, bundle / "runtime" / "config" / PRODUCTION_CONFIG.name)
        shutil.copytree(ui_dist, bundle / "runtime" / "web")

    shutil.rmtree(out, ignore_errors=True)
    staging.rename(out)


def ask(prompt: str, default: str) -> str:
    if not sys.stdin.isatty():
        return default
    answer = input(f"{prompt} [{default}]: ").strip()
    return answer or default


def confirm(prompt: str, default: bool = True) -> bool:
    if not sys.stdin.isatty():
        return default
    hint = "Y/n" if default else "y/N"
    while True:
        answer = input(f"{prompt} [{hint}]: ").strip().lower()
        if not answer:
            return default
        if answer in ("y", "yes", "n", "no"):
            return answer.startswith("y")


def ask_version() -> str:
    current = current_version()
    while True:
        ver = ask("Version", current).removeprefix("v")
        if SEMVER_RE.fullmatch(ver):
            return ver
        print('  version must look like "1.2.0"')


def ask_targets() -> list:
    choices = [[native_target()], *([t] for t in RELEASE_TARGETS), RELEASE_TARGETS]
    labels = [f"this machine ({native_target()})", *RELEASE_TARGETS, "all of the above", "other"]
    if sys.stdin.isatty():
        print("Target platform:")
        for i, label in enumerate(labels):
            print(f"  {i}) {label}")
    while True:
        choice = ask("Choose", "4")
        if choice.isdigit() and int(choice) < len(choices):
            return choices[int(choice)]
        if choice == str(len(choices)):
            target = ask("os/arch (e.g. linux/arm)", "linux/amd64")
            if target.count("/") == 1 and all(target.split("/")):
                return [target]
            print('  target must look like "linux/amd64"')


def main() -> int:
    try:
        ver = ask_version()
        targets = ask_targets()
        out = Path(ask("Output directory", str(ROOT / "dist"))).expanduser().resolve()
        with_tests = confirm("Run tests?")

        print(f"\n  version:  v{ver}\n  targets:  {', '.join(targets)}\n  output:   {out}\n  tests:    {'yes' if with_tests else 'no'}\n")
        if not confirm("Build with these settings?"):
            print("aborted")
            return 1
    except (KeyboardInterrupt, EOFError):
        print("\naborted")
        return 1
    except BuildError as err:
        print(f"error: {err}", file=sys.stderr)
        return 1

    try:
        check_output_dir(out)
        if not PRODUCTION_CONFIG.is_file():
            raise BuildError(f"{PRODUCTION_CONFIG} is missing")
        set_version(ver)
        go = tool("go")
        if with_tests:
            test_go(go)
        ui_dist = build_ui()

        with tempfile.TemporaryDirectory() as tmp:
            binaries = {target: build_binary(go, target, Path(tmp), ver) for target in targets}
            assemble(out, binaries, ui_dist)
    except BuildError as err:
        print(f"error: {err}", file=sys.stderr)
        return 1
    except KeyboardInterrupt:
        print("\naborted", file=sys.stderr)
        return 1

    step("Done")
    for bundle in sorted(out.iterdir()):
        binary = next(f for f in bundle.iterdir() if f.is_file())
        print(f"    {binary} ({binary.stat().st_size / (1 << 20):.1f} MB)")
    print("    on the server: ./moonshed install -e production   (or ./moonshed serve -e production)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
