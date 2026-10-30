#!/usr/bin/env python3
"""Run isolated probes against the real packages using a temporary Go overlay.

No source rewriting, production configuration, or external services. Fixtures
supply fake SQL/Redis connections; application initialization is explicit.
"""
import json
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]
FIXTURES = Path(__file__).resolve().parent


def main():
    with tempfile.TemporaryDirectory(prefix="auth-security-probes-") as directory:
        work = Path(directory)
        replacements = {}
        for fixture in FIXTURES.glob("*.go.fixture"):
            destination = fixture.name.removesuffix(".fixture")
            package = destination.split("_", 1)[0]
            package = {"internal": "", "admin": "http/admin"}.get(package, package)
            replacements[str(ROOT / "internal" / package / destination)] = str(fixture)
        overlay = work / "overlay.json"
        overlay.write_text(json.dumps({"Replace": replacements}))
        env = dict(os.environ, GIN_MODE="release", GOPROXY="off", GOSUMDB="off", GOWORK="off", GOTOOLCHAIN="local")
        env.setdefault("GOCACHE", str(work / "gocache"))
        print("Isolated probes against production packages via Go overlay.\n"
              "No production config, network, PostgreSQL or Redis.\n"
              "Exact project go.mod/go.sum; race detector enabled.", flush=True)
        return subprocess.run([
            "go", "test", "-overlay", str(overlay), "-mod=readonly", "-race",
            "-count=1", "-v", "./...",
        ], cwd=ROOT, env=env).returncode


if __name__ == "__main__":
    raise SystemExit(main())
