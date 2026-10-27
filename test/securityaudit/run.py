#!/usr/bin/env python3
"""Offline, isolated security regression probes; never opens production services.

Copies the audited source files verbatim into a disposable module. Only process
initialization is replaced: the DB is supplied by a fake driver and Redis by an
in-memory command hook. Uses the project's exact go.mod/go.sum, offline.
"""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]
FIXTURES = Path(__file__).resolve().parent


def main():
    with tempfile.TemporaryDirectory(prefix="auth-security-probes-") as directory:
        work = Path(directory)
        files = [
            "internal/const.go",
            "internal/password/password.go",
            "internal/util/digest.go",
            "internal/util/tg.go",
            "internal/util/mapstructure.go",
            "internal/store/user.go",
            "internal/store/client.go",
            "internal/store/admin.go",
            "internal/store/token.go",
            "internal/oauth2/manager.go",
            "internal/oauth2/server.go",
            "internal/oauth2/token.go",
            "internal/oauth2/client_policy.go",
            "internal/oauth2/jwt_access.go",
            "internal/http/introspect.go",
            "internal/http/userinfo.go",
            "internal/http/client_auth.go",
            "internal/http/middleware.go",
            "internal/http/authorize_session.go",
            "internal/http/csrf.go",
            "internal/http/session.go",
            "internal/http/rate_limit.go",
            "internal/http/logout.go",
            "internal/http/login.go",
            "internal/http/registration.go",
            "internal/http/http.go",
            "internal/http/auth.go",
            "internal/http/authorize.go",
            "internal/http/admin/api.go",
            "internal/http/admin/secret.go",
            "internal/render/render.go",
        ]
        files += [str(p.relative_to(ROOT)) for p in (ROOT / "internal/model").glob("*.go") if not p.name.endswith("_test.go")]
        for name in files:
            target = work / name
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(ROOT / name, target)
        config_source = (ROOT / "internal/config.go").read_text()
        config_types = config_source.split("type JWTConfig struct", 1)[1].split("func init()", 1)[0]
        (work / "internal/audit_environment.go").write_text('''package internal
import "fmt"
import "log/slog"
import "os"
import "reflect"
import "regexp"
import "github.com/jmoiron/sqlx"
import "github.com/redis/go-redis/v9"
var DB *sqlx.DB
var Rdb *redis.Client
var AuthServerConfig Config
type JWTConfig struct''' + config_types + config_source[config_source.index('func resolveEnv('):])
        # Production templates are parsed relative to each test package's CWD.
        for package in ("http", "http/admin", "store", "oauth2"):
            shutil.copytree(ROOT / "template", work / "internal" / package / "template")
        (work / "internal/oauth2/audit_keys.go").write_text('''package oauth2
import "auth-server/internal/model"
var JWTKeys []model.JWTKey
''')
        for fixture in FIXTURES.glob("*.go.fixture"):
            destination = fixture.name.removesuffix(".fixture")
            package = destination.split("_", 1)[0]
            if package == "internal":
                package = ""
            if package == "admin":
                package = "http/admin"
            target = work / "internal" / package / destination
            target.write_text(fixture.read_text())
        shutil.copyfile(ROOT / "go.mod", work / "go.mod")
        shutil.copyfile(ROOT / "go.sum", work / "go.sum")
        env = dict(os.environ, GIN_MODE="release", GOPROXY="off", GOSUMDB="off", GOWORK="off", GOTOOLCHAIN="local")
        # Use a writable copy when the host module cache is mounted read-only.
        if "GOMODCACHE" not in env:
            original = Path(subprocess.check_output(["go", "env", "GOMODCACHE"], text=True).strip())
            cache = work / "modcache"
            shutil.copytree(original / "cache/download", cache / "cache/download")
            env["GOMODCACHE"] = str(cache)
        env.setdefault("GOCACHE", str(work / "gocache"))
        print("Isolated probes: no production config, network, PostgreSQL or Redis.\n"
              "Exact project go.mod/go.sum; Redis commands use an in-memory hook.\n"
              "FAIL means an asserted security property was violated.", flush=True)
        result = subprocess.run(["go", "test", "-mod=mod", "-race", "-count=1", "-v", "./internal", "./internal/store", "./internal/oauth2", "./internal/http", "./internal/http/admin"], cwd=work, env=env)
        return result.returncode


if __name__ == "__main__":
    raise SystemExit(main())
