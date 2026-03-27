#!/usr/bin/env python3
"""
Regenerate charts/kmortem/templates/clusterrole.yaml from the controller-gen output.

controller-gen writes RBAC rules derived from Go marker comments into
config/rbac/role.yaml. This script extracts the rules: section and
splices it into the Helm template, which owns the metadata (name, labels,
rbac.create guard).

Run via:  make sync-helm-rbac
          (also called automatically by: make manifests)
"""

import pathlib
import sys

REPO_ROOT = pathlib.Path(__file__).parent.parent
SRC = REPO_ROOT / "config" / "rbac" / "role.yaml"
DST = REPO_ROOT / "charts" / "kmortem" / "templates" / "clusterrole.yaml"

HELM_HEADER = """\
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: {{ include "kmortem.fullname" . }}-role
  labels:
    {{- include "kmortem.labels" . | nindent 4 }}
"""


def main() -> None:
    if not SRC.exists():
        print(f"error: source role not found: {SRC}", file=sys.stderr)
        print("Run 'make manifests' to generate it first.", file=sys.stderr)
        sys.exit(1)

    src_text = SRC.read_text()

    # controller-gen always emits 'rules:' as a top-level key at column 0.
    # We rely on it being the first occurrence in the file, which holds as long
    # as the generator doesn't add description fields containing 'rules:'.
    rules_idx = src_text.find("\nrules:")
    if rules_idx == -1:
        print(f"error: no 'rules:' section found in {SRC}", file=sys.stderr)
        sys.exit(1)

    rules_section = src_text[rules_idx + 1:].rstrip()  # +1 to skip the leading newline

    out = HELM_HEADER + rules_section + "\n"
    DST.write_text(out)
    print(f"synced: {SRC.relative_to(REPO_ROOT)} → {DST.relative_to(REPO_ROOT)}")


if __name__ == "__main__":
    main()
