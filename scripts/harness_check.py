#!/usr/bin/env python3
"""Validate the repository knowledge harness and stable Python boundaries."""

from __future__ import annotations

import ast
import re
import sys
from collections import deque
from pathlib import Path
from urllib.parse import unquote


ROOT = Path(__file__).resolve().parents[1]

REQUIRED_FILES = (
    "AGENTS.md",
    "CLAUDE.md",
    "backend/pyproject.toml",
    "backend/uv.lock",
    "docs/README.md",
    "docs/architecture/overview.md",
    "docs/product/feature-map.md",
    "docs/development/standards.md",
    "docs/development/workflow.md",
    "docs/development/commands.md",
    "docs/quality/testing.md",
    "docs/quality/security.md",
    "docs/operations/runtime.md",
    "docs/harness/principles.md",
    "docs/plans/README.md",
    "docs/plans/template.md",
    "docs/plans/active/README.md",
    "docs/plans/completed/README.md",
    "docs/plans/tech-debt.md",
    "docs/decisions/README.md",
    "docs/decisions/0000-template.md",
)

FORBIDDEN_IMPORTS = {
    "domain": ("app.api", "app.application", "app.infrastructure", "app.worker"),
    "core": ("app.api", "app.application", "app.infrastructure", "app.worker"),
    "infrastructure": ("app.api", "app.application", "app.worker"),
    "application": ("app.api", "app.worker"),
}

LINK_PATTERN = re.compile(r"(?<!!)\[[^\]]+\]\(([^)]+)\)")


def relative(path: Path) -> str:
    return path.relative_to(ROOT).as_posix()


def markdown_targets(path: Path) -> list[Path]:
    targets: list[Path] = []
    for raw_target in LINK_PATTERN.findall(path.read_text(encoding="utf-8")):
        target = raw_target.strip().strip("<>")
        if target.startswith(("#", "http://", "https://", "mailto:")):
            continue
        target = unquote(target.split("#", 1)[0].split("?", 1)[0])
        if not target:
            continue
        targets.append((path.parent / target).resolve())
    return targets


def check_required_files(errors: list[str]) -> None:
    for name in REQUIRED_FILES:
        if not (ROOT / name).is_file():
            errors.append(f"missing required harness file: {name}")


def check_entrypoints(errors: list[str]) -> None:
    agents = ROOT / "AGENTS.md"
    claude = ROOT / "CLAUDE.md"
    if not agents.is_file() or not claude.is_file():
        return

    agents_text = agents.read_text(encoding="utf-8")
    claude_text = claude.read_text(encoding="utf-8")
    agents_lines = len(agents_text.splitlines())
    claude_lines = len(claude_text.splitlines())

    if agents_lines > 120:
        errors.append(f"AGENTS.md has {agents_lines} lines; keep the entry map at or below 120")
    if claude_lines > 20:
        errors.append(f"CLAUDE.md has {claude_lines} lines; keep tool-specific glue at or below 20")
    if "docs/README.md" not in agents_text:
        errors.append("AGENTS.md must route agents through docs/README.md")
    if not any(line.strip() == "@AGENTS.md" for line in claude_text.splitlines()):
        errors.append("CLAUDE.md must import @AGENTS.md as the shared instruction source")


def check_markdown(errors: list[str]) -> tuple[int, dict[Path, list[Path]]]:
    markdown_files = sorted(ROOT.glob("*.md")) + sorted((ROOT / "docs").rglob("*.md"))
    graph: dict[Path, list[Path]] = {}

    for path in markdown_files:
        targets = markdown_targets(path)
        graph[path.resolve()] = targets
        for target in targets:
            if not target.exists():
                errors.append(f"broken local link in {relative(path)}: {relative(target)}")

    docs_root = (ROOT / "docs/README.md").resolve()
    reachable: set[Path] = set()
    queue: deque[Path] = deque([docs_root])
    while queue:
        current = queue.popleft()
        if current in reachable:
            continue
        reachable.add(current)
        for target in graph.get(current, []):
            candidate = target
            if candidate.is_dir():
                candidate = candidate / "README.md"
            if candidate.suffix == ".md" and candidate in graph and candidate not in reachable:
                queue.append(candidate)

    for path in sorted((ROOT / "docs").rglob("*.md")):
        if path.resolve() not in reachable:
            errors.append(f"orphan documentation is not reachable from docs/README.md: {relative(path)}")

    return len(markdown_files), graph


def imported_modules(tree: ast.AST) -> list[tuple[str, int]]:
    modules: list[tuple[str, int]] = []
    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            modules.extend((alias.name, node.lineno) for alias in node.names)
        elif isinstance(node, ast.ImportFrom) and node.module:
            modules.append((node.module, node.lineno))
    return modules


def check_python_boundaries(errors: list[str]) -> int:
    app_root = ROOT / "backend/app"
    python_files = sorted(app_root.rglob("*.py"))
    for path in python_files:
        layer = path.relative_to(app_root).parts[0]
        forbidden = FORBIDDEN_IMPORTS.get(layer)
        if not forbidden:
            continue
        try:
            tree = ast.parse(path.read_text(encoding="utf-8"), filename=str(path))
        except SyntaxError as exc:
            errors.append(f"cannot parse {relative(path)} for boundary checks: {exc}")
            continue
        for module, line in imported_modules(tree):
            if any(module == prefix or module.startswith(f"{prefix}.") for prefix in forbidden):
                errors.append(
                    f"forbidden {layer} dependency in {relative(path)}:{line}: import {module}"
                )
    return len(python_files)


def check_ci_and_commands(errors: list[str]) -> None:
    ci_path = ROOT / ".github/workflows/ci.yml"
    makefile = ROOT / "Makefile"
    if not ci_path.is_file() or "scripts/harness_check.py" not in ci_path.read_text(encoding="utf-8"):
        errors.append(".github/workflows/ci.yml must run scripts/harness_check.py")
    if not makefile.is_file():
        errors.append("missing root Makefile command entrypoint")
        return
    make_text = makefile.read_text(encoding="utf-8")
    for target in (
        "harness-check:",
        "backend-sync:",
        "backend-check:",
        "frontend-check:",
        "check:",
    ):
        if target not in make_text:
            errors.append(f"root Makefile is missing target {target[:-1]}")

    uv_surfaces = {
        "root Makefile": make_text,
        "backend Dockerfile": (ROOT / "backend/Dockerfile").read_text(encoding="utf-8"),
        "backend CI": ci_path.read_text(encoding="utf-8"),
    }
    for label, contents in uv_surfaces.items():
        if "sync --locked" not in contents:
            errors.append(f"{label} must install backend dependencies with uv sync --locked")
    if (ROOT / "backend/requirements.txt").exists():
        errors.append("backend/requirements.txt duplicates uv project metadata; use pyproject.toml + uv.lock")


def main() -> int:
    errors: list[str] = []
    check_required_files(errors)
    check_entrypoints(errors)
    markdown_count, _ = check_markdown(errors)
    python_count = check_python_boundaries(errors)
    check_ci_and_commands(errors)

    if errors:
        print("Harness check failed:")
        for error in errors:
            print(f"- {error}")
        return 1

    print(
        "Harness check passed: "
        f"{markdown_count} Markdown files linked, {python_count} backend Python files checked."
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
