#!/usr/bin/env python3
"""Validate the repository knowledge harness and stable Python boundaries."""

from __future__ import annotations

import ast
import json
import re
import sys
from collections import deque
from pathlib import Path
from urllib.parse import unquote


ROOT = Path(__file__).resolve().parents[1]

REQUIRED_FILES = (
    "AGENTS.md",
    "CLAUDE.md",
    "backend/alembic/versions/202607100002_repair_auth_schema.py",
    "backend/pyproject.toml",
    "backend/pi-agent-runtime/package-lock.json",
    "backend/pi-agent-runtime/package.json",
    "backend/pi-agent-runtime/src/index.mjs",
    "backend/pi-agent-runtime/src/runtime.mjs",
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
    "docs/decisions/0001-use-uv-for-python-dependencies.md",
    "docs/decisions/0002-integrate-pi-as-constrained-runner.md",
    "docs/decisions/0003-enable-pi-agent-by-default.md",
)

FORBIDDEN_IMPORTS = {
    "domain": ("app.api", "app.application", "app.infrastructure", "app.worker"),
    "core": ("app.api", "app.application", "app.infrastructure", "app.worker"),
    "infrastructure": ("app.api", "app.application", "app.worker"),
    "application": ("app.api", "app.worker"),
}

DOMAIN_FORBIDDEN_EXTERNAL_IMPORTS = (
    "alembic",
    "asyncpg",
    "celery",
    "cryptography",
    "fastapi",
    "httpx",
    "langchain",
    "langgraph",
    "litellm",
    "redis",
    "sqlalchemy",
    "sqlmodel",
    "starlette",
    "structlog",
    "telegram",
    "telethon",
)

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


def matches_module(module: str, prefixes: tuple[str, ...]) -> bool:
    return any(module == prefix or module.startswith(f"{prefix}.") for prefix in prefixes)


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
            if matches_module(module, forbidden):
                errors.append(
                    f"forbidden {layer} dependency in {relative(path)}:{line}: import {module}"
                )
            if layer == "domain" and matches_module(module, DOMAIN_FORBIDDEN_EXTERNAL_IMPORTS):
                errors.append(
                    f"forbidden domain framework dependency in {relative(path)}:{line}: "
                    f"import {module}"
                )
    return len(python_files)


def assignment_literal(tree: ast.Module, name: str) -> tuple[bool, object]:
    for node in tree.body:
        value: ast.expr | None = None
        if isinstance(node, ast.AnnAssign) and isinstance(node.target, ast.Name):
            if node.target.id == name:
                value = node.value
        elif isinstance(node, ast.Assign):
            if any(isinstance(target, ast.Name) and target.id == name for target in node.targets):
                value = node.value
        if value is not None:
            try:
                return True, ast.literal_eval(value)
            except (ValueError, TypeError):
                return False, None
    return False, None


def check_migration_graph(errors: list[str], versions_root: Path | None = None) -> None:
    versions_root = versions_root or ROOT / "backend/alembic/versions"
    revisions: dict[str, tuple[Path, list[str]]] = {}

    for path in sorted(versions_root.glob("*.py")):
        if path.name == "__init__.py":
            continue
        try:
            tree = ast.parse(path.read_text(encoding="utf-8"), filename=str(path))
        except SyntaxError as exc:
            errors.append(f"cannot parse Alembic migration {relative(path)}: {exc}")
            continue

        found_revision, revision_value = assignment_literal(tree, "revision")
        found_parent, parent_value = assignment_literal(tree, "down_revision")
        if not found_revision or not isinstance(revision_value, str):
            errors.append(f"Alembic migration has no literal revision: {relative(path)}")
            continue
        if not found_parent:
            errors.append(f"Alembic migration has no literal down_revision: {relative(path)}")
            continue

        if parent_value is None:
            parents: list[str] = []
        elif isinstance(parent_value, str):
            parents = [parent_value]
        elif isinstance(parent_value, (tuple, list)) and all(
            isinstance(parent, str) for parent in parent_value
        ):
            parents = list(parent_value)
        else:
            errors.append(f"unsupported down_revision in {relative(path)}: {parent_value!r}")
            continue

        if revision_value in revisions:
            other_path = revisions[revision_value][0]
            errors.append(
                f"duplicate Alembic revision {revision_value}: "
                f"{relative(other_path)} and {relative(path)}"
            )
            continue
        revisions[revision_value] = (path, parents)

    if not revisions:
        errors.append("no Alembic migrations found")
        return

    referenced_parents: set[str] = set()
    roots: list[str] = []
    for revision, (path, parents) in revisions.items():
        if not parents:
            roots.append(revision)
        for parent in parents:
            referenced_parents.add(parent)
            if parent not in revisions:
                errors.append(
                    f"Alembic migration {relative(path)} references missing revision {parent}"
                )

    heads = sorted(set(revisions) - referenced_parents)
    if len(roots) != 1:
        errors.append(f"Alembic graph must have exactly one root, found: {sorted(roots)}")
    if len(heads) != 1:
        errors.append(f"Alembic graph must have exactly one head, found: {heads}")

    repair_revision = revisions.get("202607100002")
    expected_repair = versions_root / "202607100002_repair_auth_schema.py"
    if repair_revision is None or repair_revision[0] != expected_repair:
        errors.append("required production repair revision 202607100002 is missing or renamed")


def call_name(node: ast.AST) -> str | None:
    if isinstance(node, ast.Name):
        return node.id
    if isinstance(node, ast.Attribute):
        return node.attr
    return None


def is_timezone_datetime_call(node: ast.AST) -> bool:
    if not isinstance(node, ast.Call) or call_name(node.func) != "DateTime":
        return False
    return any(
        keyword.arg == "timezone"
        and isinstance(keyword.value, ast.Constant)
        and keyword.value.value is True
        for keyword in node.keywords
    )


def field_has_timezone_type(node: ast.AST) -> bool:
    if not isinstance(node, ast.Call) or call_name(node.func) != "Field":
        return False
    for keyword in node.keywords:
        if keyword.arg not in {"sa_type", "sa_column"}:
            continue
        if any(is_timezone_datetime_call(candidate) for candidate in ast.walk(keyword.value)):
            return True
    return False


def check_model_datetime_columns(errors: list[str], models_path: Path | None = None) -> None:
    models_path = models_path or ROOT / "backend/app/infrastructure/db/models.py"
    try:
        tree = ast.parse(models_path.read_text(encoding="utf-8"), filename=str(models_path))
    except SyntaxError as exc:
        errors.append(f"cannot parse models for timezone checks: {exc}")
        return

    for class_node in (node for node in tree.body if isinstance(node, ast.ClassDef)):
        for node in class_node.body:
            if not isinstance(node, ast.AnnAssign) or not isinstance(node.target, ast.Name):
                continue
            if not any(
                isinstance(annotation, ast.Name) and annotation.id == "datetime"
                for annotation in ast.walk(node.annotation)
            ):
                continue
            if node.value is None or not field_has_timezone_type(node.value):
                errors.append(
                    f"persistent datetime must declare DateTime(timezone=True): "
                    f"{class_node.name}.{node.target.id}"
                )


def check_ci_and_commands(errors: list[str]) -> None:
    ci_path = ROOT / ".github/workflows/ci.yml"
    docker_path = ROOT / ".github/workflows/docker.yml"
    deploy_path = ROOT / ".github/workflows/deploy.yml"
    makefile = ROOT / "Makefile"
    ci_text = ci_path.read_text(encoding="utf-8") if ci_path.is_file() else ""
    if not ci_path.is_file() or not any(
        command in ci_text for command in ("scripts/harness_check.py", "make harness-check")
    ):
        errors.append(".github/workflows/ci.yml must run the repository harness check")
    if not makefile.is_file():
        errors.append("missing root Makefile command entrypoint")
        return
    make_text = makefile.read_text(encoding="utf-8")
    for target in (
        "harness-check:",
        "backend-sync:",
        "backend-check:",
        "pi-agent-check:",
        "frontend-check:",
        "check:",
    ):
        if target not in make_text:
            errors.append(f"root Makefile is missing target {target[:-1]}")

    uv_surfaces = {
        "root Makefile": make_text,
        "backend Dockerfile": (ROOT / "backend/Dockerfile").read_text(encoding="utf-8"),
        "backend CI": ci_text,
    }
    for label, contents in uv_surfaces.items():
        if "sync --locked" not in contents:
            errors.append(f"{label} must install backend dependencies with uv sync --locked")
    if (ROOT / "backend/requirements.txt").exists():
        errors.append("backend/requirements.txt duplicates uv project metadata; use pyproject.toml + uv.lock")

    pi_package_path = ROOT / "backend/pi-agent-runtime/package.json"
    pi_lock_path = ROOT / "backend/pi-agent-runtime/package-lock.json"
    if pi_package_path.is_file() and pi_lock_path.is_file():
        try:
            pi_package = json.loads(pi_package_path.read_text(encoding="utf-8"))
            pi_lock = json.loads(pi_lock_path.read_text(encoding="utf-8"))
        except json.JSONDecodeError as exc:
            errors.append(f"cannot parse pi agent package metadata: {exc}")
        else:
            dependencies = pi_package.get("dependencies", {})
            for package in ("@earendil-works/pi-agent-core", "@earendil-works/pi-ai"):
                version = dependencies.get(package)
                if not isinstance(version, str) or not re.fullmatch(r"\d+\.\d+\.\d+", version):
                    errors.append(f"{package} must use an exact reviewed version in pi agent runtime")
            lock_root = pi_lock.get("packages", {}).get("", {})
            if lock_root.get("dependencies") != dependencies:
                errors.append("pi agent package.json and package-lock.json root dependencies differ")

    docker_text = (ROOT / "backend/Dockerfile").read_text(encoding="utf-8")
    if "FROM node:22" not in docker_text or "npm ci --omit=dev --ignore-scripts" not in docker_text:
        errors.append("backend Dockerfile must install the pinned pi agent runtime with Node 22")
    if "backend/pi-agent-runtime" not in ci_text or "npm ci --ignore-scripts" not in ci_text:
        errors.append("backend CI must validate the locked pi agent runtime")

    pi_default_surfaces = {
        "Python settings": (
            ROOT / "backend/app/core/config.py",
            "pi_agent_enabled: bool = True",
        ),
        "env example": (ROOT / "backend/.env.example", "PI_AGENT_ENABLED=true"),
        "local Compose": (ROOT / "backend/docker-compose.yml", 'PI_AGENT_ENABLED: "true"'),
        "production Compose": (
            ROOT / "backend/docker-compose.prod.yml",
            "PI_AGENT_ENABLED: ${PI_AGENT_ENABLED:-true}",
        ),
    }
    for label, (path, expected) in pi_default_surfaces.items():
        if expected not in path.read_text(encoding="utf-8"):
            errors.append(f"{label} must enable pi agent by default with: {expected}")
    deploy_text = deploy_path.read_text(encoding="utf-8") if deploy_path.is_file() else ""
    if "secrets.DEEPSEEK_API_KEY" not in deploy_text:
        errors.append("deploy workflow must accept the DeepSeek API key secret")

    release_pattern = 'branches: ["release/v*.*.*"]'
    workflow_triggers = {
        "CI": (ci_path, "push:"),
        "Build Images": (docker_path, "workflow_run:"),
        "Deploy to VPS": (deploy_path, "workflow_run:"),
    }
    for label, (path, required_event) in workflow_triggers.items():
        if not path.is_file():
            errors.append(f"missing GitHub workflow: {relative(path)}")
            continue
        trigger_block = path.read_text(encoding="utf-8").split("permissions:", 1)[0]
        if required_event not in trigger_block or release_pattern not in trigger_block:
            errors.append(
                f"{label} must be gated by {required_event.rstrip(':')} on release/v*.*.*"
            )
        forbidden_events = {"workflow_dispatch:", "pull_request:", "tags:"}
        if label != "CI":
            forbidden_events.add("push:")
        for event in forbidden_events:
            if event in trigger_block:
                errors.append(f"{label} has disallowed trigger {event.rstrip(':')}")


def main() -> int:
    errors: list[str] = []
    check_required_files(errors)
    check_entrypoints(errors)
    markdown_count, _ = check_markdown(errors)
    python_count = check_python_boundaries(errors)
    check_migration_graph(errors)
    check_model_datetime_columns(errors)
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
