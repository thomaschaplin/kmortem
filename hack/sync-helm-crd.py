#!/usr/bin/env python3
"""
Regenerate charts/kmortem/templates/crd.yaml from the controller-gen output.

Reads all kmortem.io_*.yaml files in config/crd/bases/ (source of truth),
injects the three Helm hook annotations required for pre-install/pre-upgrade
CRD management, and wraps all documents with a single installCRDs value guard.

Run via:  make sync-helm-crds
          (also called automatically by: make manifests)
"""

import pathlib
import sys

REPO_ROOT = pathlib.Path(__file__).parent.parent
SRC_DIR = REPO_ROOT / "config" / "crd" / "bases"
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


def process_crd(src: pathlib.Path) -> list[str]:
    """Read a CRD file and inject Helm annotations. Returns the output lines."""
    lines = src.read_text().splitlines()

    injected = False
    out: list[str] = []
    for line in lines:
        out.append(line)
        if INJECT_AFTER in line:
            out.extend(HELM_ANNOTATIONS)
            injected = True

    if not injected:
        print(
            f"error: injection marker '{INJECT_AFTER}' not found in {src}",
            file=sys.stderr,
        )
        print("The controller-gen annotation format may have changed.", file=sys.stderr)
        sys.exit(1)

    # Strip trailing blank lines from this document.
    while out and not out[-1].strip():
        out.pop()

    return out


def main() -> None:
    srcs = sorted(SRC_DIR.glob("kmortem.io_*.yaml"))
    if not srcs:
        print(f"error: no CRD files found in {SRC_DIR}", file=sys.stderr)
        print("Run 'make manifests' to generate them first.", file=sys.stderr)
        sys.exit(1)

    all_lines = ["{{- if .Values.installCRDs }}"]
    for i, src in enumerate(srcs):
        doc_lines = process_crd(src)
        if i > 0:
            # Separate documents with a blank line.
            all_lines.append("")
        all_lines.extend(doc_lines)
        print(f"synced: {src.relative_to(REPO_ROOT)} → {DST.relative_to(REPO_ROOT)}")

    all_lines.append("{{- end }}")

    DST.write_text("\n".join(all_lines) + "\n")


if __name__ == "__main__":
    main()
