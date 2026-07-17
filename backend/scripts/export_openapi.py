"""Export the FastAPI schema deterministically for TypeScript contract generation."""

from __future__ import annotations

import argparse
import json
import os
import sys
from pathlib import Path

# Schema generation must not depend on a developer's secrets or a running database.
os.environ.setdefault(
    "DATABASE_URL",
    "postgresql+asyncpg://openapi:openapi@127.0.0.1/openapi",
)
os.environ.setdefault("ADMIN_API_TOKEN", "openapi-generation-only")
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from app.main import create_app  # noqa: E402


def rendered_schema() -> str:
    return json.dumps(create_app().openapi(), ensure_ascii=False, indent=2, sort_keys=True) + "\n"


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("output", type=Path)
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()

    content = rendered_schema()
    if args.check:
        if not args.output.exists() or args.output.read_text() != content:
            raise SystemExit(f"OpenAPI snapshot is stale: {args.output}")
        return

    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(content)


if __name__ == "__main__":
    main()
