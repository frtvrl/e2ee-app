#!/usr/bin/env python3
"""End-to-end env-var expansion simulation for infra/docker-compose.yml.

Docker is not installed on this machine, so we can't run `docker compose config`.
But Compose's variable-substitution rules are simple enough to simulate in Python:

  ${VAR}            -> VAR's value, or "" if unset
  ${VAR:-default}   -> VAR's value, or default if unset/empty
  ${VAR:?message}   -> raises ValueError with message if unset/empty
  ${VAR:+alt}       -> alt if VAR is set, else ""

This script:
  1. Loads infra/.env.example into a dict (mocking production .env).
  2. Walks the parsed YAML, replacing every string that contains ${...}.
  3. Verifies the required env vars (those with ${VAR:?msg}) raise a clear
     error if missing — exactly what Compose would do.

Run:
    python infra/scripts/validate_env.py
"""

from __future__ import annotations

import os
import re
import sys
from pathlib import Path

import yaml


# Match ${VAR}, ${VAR:-default}, ${VAR:?error}, ${VAR:+alt}
# We deliberately use a non-greedy match and capture the operator.
ENV_RE = re.compile(r"\$\{([A-Za-z_][A-Za-z0-9_]*)((:-|:\\?\?|\+)[^}]*)?\}")


def expand(value: str, env: dict[str, str]) -> str:
    """Expand ${...} placeholders. Raises ValueError for ${VAR:?msg} if missing."""

    def repl(m: re.Match) -> str:
        var = m.group(1)
        op = m.group(2)
        v = env.get(var)
        if op is None:
            return v if v is not None else ""
        # op looks like ":?-msg" or ":-default" or ":+alt"
        # The first char is ':', the second is the operator ('?'/'-'/'+').
        op_kind = op[1] if len(op) > 1 else ""
        arg = op[2:] if len(op) > 2 else ""
        if op_kind == "-":
            return v if v not in (None, "") else arg
        if op_kind == "?":
            if v in (None, ""):
                raise ValueError(f"required env var {var!r} unset: {arg}")
            return v
        if op_kind == "+":
            return arg if v not in (None, "") else ""
        # unreachable
        return m.group(0)

    return ENV_RE.sub(repl, value)


def walk(obj, env):
    """Recursively expand strings inside obj (in place where mutable)."""
    if isinstance(obj, dict):
        return {k: walk(v, env) for k, v in obj.items()}
    if isinstance(obj, list):
        return [walk(v, env) for v in obj]
    if isinstance(obj, str):
        return expand(obj, env)
    return obj


def load_env_file(path: Path) -> dict[str, str]:
    """Tiny .env parser: KEY=VALUE, blank lines and # comments ignored.
    Quoted values are stripped of surrounding quotes."""
    env: dict[str, str] = {}
    if not path.is_file():
        return env
    for raw in path.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        if "=" not in line:
            continue
        k, v = line.split("=", 1)
        k = k.strip()
        v = v.strip()
        # Strip surrounding quotes (single or double)
        if len(v) >= 2 and v[0] == v[-1] and v[0] in ("'", '"'):
            v = v[1:-1]
        env[k] = v
    return env


def main() -> int:
    here = Path(__file__).resolve().parent
    infra = here.parent
    compose = infra / "docker-compose.yml"
    env_example = infra / ".env.example"

    if not compose.is_file():
        print(f"ERROR: {compose} not found", file=sys.stderr)
        return 1

    raw = compose.read_text(encoding="utf-8")
    doc = yaml.safe_load(raw)

    # ------------------------------------------------------------------------
    # Test 1: Expansion with infra/.env.example (where __SET_ME__ are placeholders)
    # ------------------------------------------------------------------------
    env_full = load_env_file(env_example)
    # Compose's default env (process env + .env file). Process env wins.
    env_full = {**env_full, **dict(os.environ)}

    missing: list[str] = []
    try:
        expanded_full = walk(doc, env_full)
    except ValueError as e:
        missing.append(str(e))

    if missing:
        print("FAIL: required env vars missing in .env.example:")
        for m in missing:
            print(f"  - {m}")
        print()
        print("(Hint: copy infra/.env.example to infra/.env and set __SET_ME__ placeholders.)")
        return 2

    print(f"OK: expanded {compose.name} with .env.example ({len(env_full)} vars)")
    print(f"  services after expansion: {list(expanded_full['services'].keys())}")

    # ------------------------------------------------------------------------
    # Test 2: Verify that missing required vars raise a clear error.
    # We isolate just the lines that have ${VAR:?...} and strip them from env.
    # ------------------------------------------------------------------------
    # Find every ${VAR:?...} pattern in the compose file
    required_pattern = re.compile(r"\$\{([A-Z_][A-Za-z0-9_]*):\?")
    required_vars = sorted(set(required_pattern.findall(raw)))
    print(f"  required env vars ({len(required_vars)}): {', '.join(required_vars)}")

    if not required_vars:
        print("  (no required-env-var markers; nothing to test)")
        return 0

    # Strip them one at a time and confirm expansion raises
    failures: list[str] = []
    for var in required_vars:
        # Strip only this single var
        env_one_stripped = {k: v for k, v in env_full.items() if k != var}
        try:
            walk(doc, env_one_stripped)
            failures.append(var)
        except ValueError as e:
            # Good — Compose would also error
            print(f"  -> {var!r:24s} raises when missing: {e}")

    if failures:
        print()
        print(f"FAIL: required-var check did NOT catch: {failures}", file=sys.stderr)
        return 3

    print()
    print("PASS: env-var expansion simulated (full + missing-required).")
    return 0


if __name__ == "__main__":
    sys.exit(main())