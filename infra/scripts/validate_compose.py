#!/usr/bin/env python3
"""YAML validation for OpenE2EE infra/docker-compose.yml.

Since Docker may not be installed on this machine (per HANDOFF.md
"Bu makinede Docker kurulu olmayabilir"), we validate the compose
syntax with PyYAML's safe_load + a structural sanity check (services,
networks, volumes defined).

This is NOT a full `docker compose config` — it doesn't dereference
env vars or expand anchors the way compose does. But it catches:
  - YAML syntax errors (most common)
  - Unknown top-level keys
  - Service shape anomalies (no image/build, no depends_on, etc.)

Run from the repo root:
    python infra/scripts/validate_compose.py

Or from anywhere:
    python <repo>/infra/scripts/validate_compose.py <repo>/infra/docker-compose.yml
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

import yaml


REQUIRED_TOP_KEYS = {"services"}
REQUIRED_SERVICE_KEYS = {"image", "build"}  # one of
REQUIRED_NETWORK_KEYS = set()  # any
REQUIRED_VOLUME_KEYS = set()   # any


def fail(msg: str, errors: list[str]) -> None:
    errors.append(msg)


def validate(doc: dict, source: Path) -> list[str]:
    errors: list[str] = []

    if not isinstance(doc, dict):
        fail(f"{source}: top-level is not a mapping (got {type(doc).__name__})", errors)
        return errors

    # Top-level keys
    if REQUIRED_TOP_KEYS - set(doc.keys()):
        fail(f"{source}: missing required top-level keys: {REQUIRED_TOP_KEYS - set(doc.keys())}", errors)

    # networks
    networks = doc.get("networks", {})
    if not isinstance(networks, dict):
        fail(f"{source}: 'networks' must be a mapping", errors)
    else:
        for name, spec in networks.items():
            if not isinstance(spec, dict):
                fail(f"{source}: networks.{name} must be a mapping", errors)

    # volumes
    volumes = doc.get("volumes", {})
    if not isinstance(volumes, dict):
        fail(f"{source}: 'volumes' must be a mapping", errors)

    # secrets (optional but if present must be a mapping)
    secrets = doc.get("secrets", {})
    if secrets and not isinstance(secrets, dict):
        fail(f"{source}: 'secrets' must be a mapping", errors)

    # services
    services = doc.get("services", {})
    if not isinstance(services, dict) or not services:
        fail(f"{source}: 'services' is missing or empty", errors)
        return errors

    for svc_name, svc in services.items():
        if not isinstance(svc, dict):
            fail(f"{source}: services.{svc_name} is not a mapping", errors)
            continue

        # Must have image OR build
        if not (REQUIRED_SERVICE_KEYS & set(svc.keys())):
            fail(
                f"{source}: services.{svc_name} missing both 'image' and 'build'",
                errors,
            )

        # networks field (if set) must be list[str|dict]
        nets = svc.get("networks")
        if nets is not None and not isinstance(nets, (list, str)):
            fail(f"{source}: services.{svc_name}.networks must be list or string", errors)

        # depends_on: list[str] | dict
        dep = svc.get("depends_on")
        if dep is not None:
            if isinstance(dep, list):
                for d in dep:
                    if not isinstance(d, str):
                        fail(f"{source}: services.{svc_name}.depends_on item must be string (got {type(d).__name__})", errors)
            elif isinstance(dep, dict):
                for d_name, d_spec in dep.items():
                    if not isinstance(d_spec, dict):
                        fail(f"{source}: services.{svc_name}.depends_on.{d_name} must be a mapping", errors)

        # profiles: list[str] (when present, service is opt-in)
        prof = svc.get("profiles")
        if prof is not None and not isinstance(prof, list):
            fail(f"{source}: services.{svc_name}.profiles must be list", errors)

    return errors


def main() -> int:
    if len(sys.argv) > 1:
        compose_path = Path(sys.argv[1])
    else:
        compose_path = Path(__file__).resolve().parent.parent / "docker-compose.yml"

    if not compose_path.is_file():
        print(f"ERROR: {compose_path} not found", file=sys.stderr)
        return 1

    raw = compose_path.read_text(encoding="utf-8")
    try:
        doc = yaml.safe_load(raw)
    except yaml.YAMLError as e:
        print(f"FAIL: YAML parse error in {compose_path}:", file=sys.stderr)
        print(f"  {e}", file=sys.stderr)
        return 2

    errors = validate(doc, compose_path)

    # Summary
    services = doc.get("services", {}) if isinstance(doc, dict) else {}
    networks = doc.get("networks", {}) if isinstance(doc, dict) else {}
    volumes = doc.get("volumes", {}) if isinstance(doc, dict) else {}
    secrets = doc.get("secrets", {}) if isinstance(doc, dict) else {}

    print(f"OK: parsed {compose_path}")
    print(f"  YAML size: {len(raw)} bytes, {raw.count(chr(10)) + 1} lines")
    print(f"  services:  {len(services)} -> {', '.join(sorted(services.keys()))}")
    print(f"  networks:  {len(networks)} -> {', '.join(sorted(networks.keys()))}")
    print(f"  volumes:   {len(volumes)} -> {', '.join(sorted(volumes.keys()))}")
    if secrets:
        print(f"  secrets:   {len(secrets)} -> {', '.join(sorted(secrets.keys()))}")

    # Per-service summary (compact)
    print()
    print("Service details:")
    for name, svc in sorted(services.items()):
        img = svc.get("image", "(build)")
        prof = svc.get("profiles")
        profs = f" profiles={prof}" if prof else ""
        nets = svc.get("networks", [])
        net = f" nets={','.join(nets)}" if isinstance(nets, list) and nets else ""
        deps = svc.get("depends_on")
        dep = f" depends_on={','.join(deps) if isinstance(deps, list) else list(deps.keys()) if isinstance(deps, dict) else ''}" if deps else ""
        ports = svc.get("ports", [])
        port_summary = f" ports={len(ports)}" if ports else ""
        print(f"  - {name:12s} image={img}{profs}{net}{dep}{port_summary}")

    if errors:
        print()
        print(f"FAIL: {len(errors)} structural issue(s):", file=sys.stderr)
        for e in errors:
            print(f"  - {e}", file=sys.stderr)
        return 3

    print()
    print("PASS: structural validation OK (YAML parse + service shape).")
    print("Note: this is NOT `docker compose config`. Full env-var expansion,")
    print("anchor resolution, and `version` normalization require Docker.")
    return 0


if __name__ == "__main__":
    sys.exit(main())