"""Build and cache an OpenSERP sidecar for one target platform."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess


BUILD_INPUT_SUFFIXES = {
    ".css",
    ".go",
    ".html",
    ".js",
    ".json",
    ".mod",
    ".sum",
    ".svg",
    ".tmpl",
    ".txt",
    ".yaml",
    ".yml",
}


def build_inputs(source_dir: Path) -> list[Path]:
    return sorted(
        path
        for path in source_dir.rglob("*")
        if path.is_file()
        and ".git" not in path.parts
        and (path.suffix.lower() in BUILD_INPUT_SUFFIXES or path.name in {"go.mod", "go.sum"})
    )


def toolchain_version() -> str:
    return subprocess.check_output(["go", "version"], text=True).strip()


def source_digest(source_dir: Path, goos: str, goarch: str) -> str:
    digest = hashlib.sha256()
    digest.update(f"goos={goos}\ngoarch={goarch}\n".encode())
    digest.update(f"toolchain={toolchain_version()}\n".encode())
    for path in build_inputs(source_dir):
        digest.update(path.relative_to(source_dir).as_posix().encode())
        digest.update(b"\0")
        digest.update(path.read_bytes())
        digest.update(b"\0")
    return digest.hexdigest()


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--source-dir", required=True, type=Path)
    parser.add_argument("--cache-dir", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--goos", required=True)
    parser.add_argument("--goarch", required=True)
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    source_dir = args.source_dir.resolve()
    cache_dir = args.cache_dir.resolve()
    output = args.output.resolve()
    cache_binary = cache_dir / "openserp"
    metadata_path = cache_dir / "metadata.json"
    digest = source_digest(source_dir, args.goos, args.goarch)

    cached = False
    if metadata_path.is_file() and cache_binary.is_file():
        metadata = json.loads(metadata_path.read_text(encoding="utf-8"))
        cached = metadata.get("digest") == digest

    if not cached:
        cache_dir.mkdir(parents=True, exist_ok=True)
        temporary_binary = cache_dir / "openserp.tmp"
        env = os.environ.copy()
        env.update({"GOOS": args.goos, "GOARCH": args.goarch, "CGO_ENABLED": "0"})
        subprocess.run(
            ["go", "build", "-trimpath", "-buildvcs=false", "-o", str(temporary_binary), "."],
            cwd=source_dir,
            env=env,
            check=True,
        )
        os.replace(temporary_binary, cache_binary)
        temporary_metadata = cache_dir / "metadata.json.tmp"
        temporary_metadata.write_text(
            json.dumps(
                {
                    "digest": digest,
                    "goos": args.goos,
                    "goarch": args.goarch,
                    "toolchain": toolchain_version(),
                },
                indent=2,
            )
            + "\n",
            encoding="utf-8",
        )
        os.replace(temporary_metadata, metadata_path)

    output.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(cache_binary, output)
    print(f"OpenSERP cache {'hit' if cached else 'rebuilt'}: {output}")


if __name__ == "__main__":
    main()
