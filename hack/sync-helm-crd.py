#!/usr/bin/env python3
"""
Regenerate charts/kmortem/templates/crd.yaml from the controller-gen output.

Reads  config/crd/bases/kmortem.io_nodereports.yaml (source of truth),
injects the three Helm hook annotations required for pre-install/pre-upgrade
CRD management, and wraps the document with the installCRDs value guard.

Run via:  make sync-helm-crds
          (also called automatically by: make manifests)
"""

import pathlib
import sys

REPO_ROOT = pathlib.Path(__file__).parent.parent
SRC = REPO_ROOT / "config" / "crd" / "bases" / "kmortem.io_nodereports.yaml"
DST = REPO_ROOT / "charts" / "kmortem" / "templates" / "crd.yaml"

HELM_ANNOTATIONS = [
    '    "helm.sh/hook": pre-install,pre-upgrade',
    '    "helm.sh/hook-weight": "-5"',
    # hook-failed: only clean up the hook resource when it fails.
    # Do NOT use before-hook-creation here — that would delete the CRD (and all
    # NodeReport objects) before every helm upgrade.
    '    "helm.sh/hook-delete-policy": hook-failed',
]

INJECT_AFTER = "controller-gen.kubebuilder.io/version:"


def main() -> None:
    if not SRC.exists():
        print(f"error: source CRD not found: {SRC}", file=sys.stderr)
        print("Run 'make manifests' to generate it first.", file=sys.stderr)
        sys.exit(1)

    lines = SRC.read_text().splitlines()

    injected = False
    out = ["{{- if .Values.installCRDs }}"]
    for line in lines:
        out.append(line)
        if INJECT_AFTER in line:
            out.extend(HELM_ANNOTATIONS)
            injected = True

    if not injected:
        print(
            f"error: injection marker '{INJECT_AFTER}' not found in {SRC}",
            file=sys.stderr,
        )
        print("The controller-gen annotation format may have changed.", file=sys.stderr)
        sys.exit(1)

    while out and not out[-1].strip():
        out.pop()
    out.append("{{- end }}")

    DST.write_text("\n".join(out) + "\n")
    print(f"synced: {SRC.relative_to(REPO_ROOT)} → {DST.relative_to(REPO_ROOT)}")


if __name__ == "__main__":
    main()
