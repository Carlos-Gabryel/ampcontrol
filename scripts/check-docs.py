#!/usr/bin/env python3
"""Validate bilingual documentation pairs and relative Markdown links."""

from __future__ import annotations

import os
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
ENGLISH = os.environ.get("AMPCONTROL_LANGUAGE", "").lower() in {"en", "en-us"}
LINK = re.compile(r"\]\(([^)]+)\)")
PAIRS = (
    ("README.md", "README.en.md"),
    ("SECURITY.md", "SECURITY.en.md"),
    ("CONTRIBUTING.md", "CONTRIBUTING.en.md"),
    ("THIRD_PARTY_NOTICES.md", "THIRD_PARTY_NOTICES.en.md"),
    ("docs/INSTALLATION.md", "docs/INSTALLATION.en.md"),
    ("docs/CONFIGURATION.md", "docs/CONFIGURATION.en.md"),
    ("docs/ARCHITECTURE.md", "docs/ARCHITECTURE.en.md"),
    ("config/ampcontrol.example.toml", "config/ampcontrol.example.en.toml"),
)


def message(portuguese: str, english: str) -> str:
    return english if ENGLISH else portuguese


def main() -> int:
    errors: list[str] = []
    for portuguese, english in PAIRS:
        for relative in (portuguese, english):
            if not (ROOT / relative).is_file():
                errors.append(message(f"arquivo ausente: {relative}", f"missing file: {relative}"))

    for markdown in ROOT.rglob("*.md"):
        for match in LINK.finditer(markdown.read_text(encoding="utf-8")):
            target = match.group(1).split("#", 1)[0].strip()
            if not target or target.startswith(("http://", "https://", "mailto:")):
                continue
            if not (markdown.parent / target).resolve().exists():
                errors.append(message(
                    f"link quebrado em {markdown.relative_to(ROOT)}: {target}",
                    f"broken link in {markdown.relative_to(ROOT)}: {target}",
                ))

    if errors:
        for error in errors:
            print(f"ERROR: {error}", file=sys.stderr)
        return 1
    print(message("Documentação bilíngue: OK", "Bilingual documentation: OK"))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
