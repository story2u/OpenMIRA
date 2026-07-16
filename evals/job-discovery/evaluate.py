#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import sys
from dataclasses import fields
from datetime import UTC, datetime
from decimal import Decimal
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "backend"))

from app.domain.enums import SourcePrimaryFunction  # noqa: E402
from app.domain.services.job_discovery import prefilter_job_message, profile_source  # noqa: E402
from app.domain.services.job_matching import (  # noqa: E402
    JobFacts,
    SearchPreferences,
    calculate_job_match,
)

DATA_DIR = Path(__file__).parent
FORMAL_JOB_CLASSES = {"job_post", "job_repost"}
NESTED_JOB_FIELDS = {
    "salary_raw": ("salary", "raw"),
    "salary_currency": ("salary", "currency"),
    "salary_period": ("salary", "period"),
}


def read_jsonl(path: Path) -> list[dict]:
    with path.open(encoding="utf-8") as handle:
        return [json.loads(line) for line in handle if line.strip()]


def evaluate_sources() -> dict:
    rows = read_jsonl(DATA_DIR / "source-profiling.jsonl")
    correct = 0
    for row in rows:
        result = profile_source(
            name=row["name"],
            description=row.get("description"),
            username=row.get("username"),
            samples=row.get("samples", []),
            now=datetime.now(UTC),
        )
        correct += result.primary_function.value == row["expected_primary_function"]
    return {"samples": len(rows), "accuracy": round(correct / len(rows), 4)}


def evaluate_prefilter() -> dict:
    rows = read_jsonl(DATA_DIR / "message-classification.jsonl")
    true_positive = false_positive = false_negative = 0
    filtered_class_correct = filtered_count = 0
    for row in rows:
        result = prefilter_job_message(
            row["text"],
            primary_function=SourcePrimaryFunction(row["source_function"]),
            job_signal_prior=row["job_signal_prior"],
        )
        expected_positive = row["expected_classification"] in FORMAL_JOB_CLASSES
        true_positive += result.should_analyze and expected_positive
        false_positive += result.should_analyze and not expected_positive
        false_negative += not result.should_analyze and expected_positive
        if not expected_positive:
            filtered_count += 1
            filtered_class_correct += result.classification.value == row["expected_classification"]
    precision = true_positive / max(true_positive + false_positive, 1)
    recall = true_positive / max(true_positive + false_negative, 1)
    return {
        "samples": len(rows),
        "candidate_precision": round(precision, 4),
        "candidate_recall": round(recall, 4),
        "deterministic_negative_class_accuracy": round(
            filtered_class_correct / max(filtered_count, 1), 4
        ),
        "note": "Measures deterministic prefilter routing, not Pi Agent final classification.",
    }


def _dataclass_from_dict(model, values: dict):
    allowed = {item.name for item in fields(model)}
    converted = dict(values)
    for key in ("salary_min", "salary_max", "minimum_salary"):
        if converted.get(key) is not None:
            converted[key] = Decimal(str(converted[key]))
    return model(**{key: value for key, value in converted.items() if key in allowed})


def evaluate_matching() -> dict:
    rows = read_jsonl(DATA_DIR / "job-matching.jsonl")
    correct = 0
    explanation_complete = 0
    for row in rows:
        decision = calculate_job_match(
            _dataclass_from_dict(JobFacts, row["job"]),
            _dataclass_from_dict(SearchPreferences, row["profile"]),
        )
        in_range = row["minimum_score"] <= decision.match_score <= row["maximum_score"]
        eligibility_ok = decision.eligibility.value == row["expected_eligibility"]
        unknown_ok = (
            row.get("expected_unknown") in decision.unknown_constraints
            if row.get("expected_unknown")
            else True
        )
        correct += in_range and eligibility_ok and unknown_ok
        explanation_complete += bool(
            decision.matched_reasons or decision.mismatch_reasons or decision.unknown_constraints
        )
    return {
        "samples": len(rows),
        "decision_accuracy": round(correct / len(rows), 4),
        "explanation_completeness": round(explanation_complete / len(rows), 4),
    }


def evaluate_model_predictions(predictions_path: Path | None) -> dict:
    if predictions_path is None:
        return {
            "status": "not_run",
            "reason": "Pass --predictions with reviewed Pi Agent JSONL outputs to score extraction and duplicate accuracy.",
            "fixture_samples": len(read_jsonl(DATA_DIR / "job-extraction.jsonl")),
        }
    expected = {row["id"]: row for row in read_jsonl(DATA_DIR / "job-extraction.jsonl")}
    predictions = {row["id"]: row for row in read_jsonl(predictions_path)}
    fields_correct = fields_total = evidence_correct = evidence_total = 0
    duplicate_correct = duplicate_total = 0
    for identifier, fixture in expected.items():
        prediction = predictions.get(identifier, {})
        job = prediction.get("job") or {}
        for key, value in fixture["expected"].items():
            fields_total += 1
            actual = job
            for part in NESTED_JOB_FIELDS.get(key, (key,)):
                actual = actual.get(part) if isinstance(actual, dict) else None
            fields_correct += actual == value
        evidence = prediction.get("field_evidence") or {}
        for key in fixture.get("required_evidence", []):
            evidence_total += 1
            evidence_correct += bool(evidence.get(key))
        if fixture.get("duplicate_key") is not None:
            duplicate_total += 1
            duplicate_correct += prediction.get("duplicate_key") == fixture["duplicate_key"]
        expected_flags = set(fixture.get("expected_compliance_flags", []))
        if expected_flags:
            fields_total += 1
            fields_correct += expected_flags.issubset(set(prediction.get("compliance_flags", [])))
    return {
        "status": "completed",
        "field_accuracy": round(fields_correct / max(fields_total, 1), 4),
        "evidence_completeness": round(evidence_correct / max(evidence_total, 1), 4),
        "duplicate_accuracy": (
            round(duplicate_correct / duplicate_total, 4) if duplicate_total else None
        ),
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--predictions", type=Path)
    args = parser.parse_args()
    report = {
        "dataset": "fictional-job-discovery-v1",
        "source_profiling_baseline": evaluate_sources(),
        "prefilter_baseline": evaluate_prefilter(),
        "deterministic_matching": evaluate_matching(),
        "pi_agent_extraction": evaluate_model_predictions(args.predictions),
    }
    print(json.dumps(report, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
