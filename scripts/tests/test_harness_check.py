from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

from scripts import harness_check


class HarnessCheckTests(unittest.TestCase):
    def write_migration(self, root: Path, filename: str, revision: str, parent: str | None) -> None:
        parent_literal = repr(parent)
        (root / filename).write_text(
            f'revision: str = "{revision}"\n'
            f"down_revision: str | None = {parent_literal}\n",
            encoding="utf-8",
        )

    def test_domain_framework_prefixes_are_forbidden(self) -> None:
        self.assertTrue(
            harness_check.matches_module(
                "fastapi.routing", harness_check.DOMAIN_FORBIDDEN_EXTERNAL_IMPORTS
            )
        )
        self.assertTrue(
            harness_check.matches_module(
                "sqlalchemy.orm", harness_check.DOMAIN_FORBIDDEN_EXTERNAL_IMPORTS
            )
        )
        self.assertFalse(
            harness_check.matches_module(
                "pydantic", harness_check.DOMAIN_FORBIDDEN_EXTERNAL_IMPORTS
            )
        )

    def test_migration_graph_accepts_single_required_head(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.write_migration(root, "202607100001_base.py", "202607100001", None)
            self.write_migration(
                root,
                "202607100002_repair_auth_schema.py",
                "202607100002",
                "202607100001",
            )
            errors: list[str] = []
            harness_check.check_migration_graph(errors, root)
            self.assertEqual(errors, [])

    def test_migration_graph_rejects_multiple_heads(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.write_migration(root, "202607100001_base.py", "202607100001", None)
            self.write_migration(
                root,
                "202607100002_repair_auth_schema.py",
                "202607100002",
                "202607100001",
            )
            self.write_migration(root, "202607100003_branch.py", "202607100003", "202607100001")
            errors: list[str] = []
            harness_check.check_migration_graph(errors, root)
            self.assertTrue(any("exactly one head" in error for error in errors))

    def test_migration_graph_rejects_missing_repair_revision(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.write_migration(root, "202607100001_base.py", "202607100001", None)
            errors: list[str] = []
            harness_check.check_migration_graph(errors, root)
            self.assertTrue(
                any("required production repair revision 202607100002" in error for error in errors)
            )

    def test_datetime_columns_require_explicit_timezone(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            models_path = Path(directory) / "models.py"
            models_path.write_text(
                "from datetime import datetime\n"
                "class Good:\n"
                "    created_at: datetime = Field(sa_type=DateTime(timezone=True))\n"
                "class Bad:\n"
                "    created_at: datetime = Field(default=None)\n",
                encoding="utf-8",
            )
            errors: list[str] = []
            harness_check.check_model_datetime_columns(errors, models_path)
            self.assertEqual(
                errors,
                ["persistent datetime must declare DateTime(timezone=True): Bad.created_at"],
            )

    def test_release_workflow_requires_direct_ci_trigger(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            workflow_path = Path(directory) / "deploy.yml"
            workflow_path.write_text(
                "on:\n"
                "  workflow_run:\n"
                '    workflows: ["Build Images"]\n'
                "jobs:\n"
                "  build:\n"
                "    steps:\n"
                "      - uses: docker/build-push-action@v7\n"
                "  deploy:\n"
                "    needs: build\n"
                "env:\n"
                "  DEEPSEEK_API_KEY: ${{ secrets.DEEPSEEK_API_KEY }}\n",
                encoding="utf-8",
            )
            errors: list[str] = []
            harness_check.check_release_workflow(errors, workflow_path)
            self.assertIn("deploy workflow must be triggered directly by CI", errors)
            self.assertIn("deploy workflow must not chain workflow_run from Build Images", errors)

    def test_release_workflow_stops_database_clients_before_migration(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            workflow_path = Path(directory) / "deploy.yml"
            workflow_path.write_text(
                "on:\n"
                "  workflow_run:\n"
                '    workflows: ["CI"]\n'
                "jobs:\n"
                "  build:\n"
                "    steps:\n"
                "      - uses: docker/build-push-action@v7\n"
                "  deploy:\n"
                "    needs: build\n"
                "    steps:\n"
                "      - run: docker compose --env-file .env up --no-deps --force-recreate --exit-code-from migrate migrate\n"
                "      - run: docker compose --env-file .env stop $database_client_services\n"
                "      - run: docker compose exec -T api python -m scripts.register_telegram_webhook\n"
                "env:\n"
                "  DEEPSEEK_API_KEY: ${{ secrets.DEEPSEEK_API_KEY }}\n"
                "database_client_services=api\n",
                encoding="utf-8",
            )
            errors: list[str] = []
            harness_check.check_release_workflow(errors, workflow_path)
            self.assertIn("deploy workflow must stop database clients before migration", errors)


if __name__ == "__main__":
    unittest.main()
