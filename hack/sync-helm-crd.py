#!/usr/bin/env python3
"""
Regenerate charts/kmortem/templates/crd.yaml from the controller-gen output.

Reads  config/crd/bases/kmortem.io_nodereports.yaml (source of truth)
and wraps the document with the installCRDs value guard.

The CRD is a regular chart resource (not a hook) so that Helm patches it on
upgrade rather than attempting a create, which would fail when it already exists.

Run via:  make sync-helm-crds
          (also called automatically by: make manifests)
"""

import pathlib
import sys

REPO_ROOT = pathlib.Path(__file__).parent.parent
SRC = REPO_ROOT / "config" / "crd" / "bases" / "kmortem.io_nodereports.yaml"
DST = REPO_ROOT / "charts" / "kmortem" / "templates" / "crd.yaml"


def main() -> None:
    if not SRC.exists():
        print(f"error: source CRD not found: {SRC}", file=sys.stderr)
        print("Run 'make manifests' to generate it first.", file=sys.stderr)
        sys.exit(1)

    lines = SRC.read_text().splitlines()

    out = ["{{- if .Values.installCRDs }}"]
    out.extend(lines)

    while out and not out[-1].strip():
        out.pop()
    out.append("{{- end }}")

    DST.write_text("\n".join(out) + "\n")
    print(f"synced: {SRC.relative_to(REPO_ROOT)} → {DST.relative_to(REPO_ROOT)}")


if __name__ == "__main__":
    main()
