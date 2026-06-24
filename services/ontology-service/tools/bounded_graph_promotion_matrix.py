#!/usr/bin/env python3
"""Run and validate the AI-first bounded graph promotion matrix."""

from __future__ import annotations

import argparse
import glob
import json
import os
import subprocess
import sys
from pathlib import Path
from typing import Any


TOOLS_DIR = Path(__file__).resolve().parent
ROOT_DIR = TOOLS_DIR.parent
DEFAULT_CASES_JSON = TOOLS_DIR / "eval_packs" / "bounded_graph_promotion_matrix" / "cases.json"


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Run bounded graph promotion matrix cases.")
    parser.add_argument("--cases-json", type=Path, default=DEFAULT_CASES_JSON)
    parser.add_argument("--out-dir", type=Path, default=Path("/tmp/bounded_graph_promotion_matrix"))
    parser.add_argument("--report-json", type=Path)
    parser.add_argument("--skip-run", action="store_true", help="Validate artifacts without running case commands.")
    parser.add_argument("--case", action="append", dest="case_keys", default=[], help="Run only the named case key. Repeatable.")
    parser.add_argument("--include-tag", action="append", default=[], help="Run only cases with this tag. Repeatable.")
    parser.add_argument("--require-advisory-tags", action="store_true", help="Treat missing advisory tags as blocking failures.")
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> None:
    args = parse_args(argv)
    report = evaluate_matrix(
        load_json(args.cases_json),
        out_dir=args.out_dir,
        run_commands=not args.skip_run,
        selected_case_keys=set(args.case_keys),
        include_tags=set(args.include_tag),
        require_advisory_tags=args.require_advisory_tags,
    )
    if args.report_json:
        args.report_json.parent.mkdir(parents=True, exist_ok=True)
        args.report_json.write_text(json.dumps(report, indent=2, sort_keys=True), encoding="utf-8")
    print_matrix_summary(report)
    if not report["passes_matrix"]:
        raise SystemExit(format_failures(report))


def evaluate_matrix(
    config: dict[str, Any],
    *,
    out_dir: Path,
    run_commands: bool,
    selected_case_keys: set[str] | None = None,
    include_tags: set[str] | None = None,
    require_advisory_tags: bool = False,
) -> dict[str, Any]:
    selected_case_keys = selected_case_keys or set()
    include_tags = include_tags or set()
    matrix_key = clean(config.get("matrix_key")) or "bounded_graph_promotion_matrix"
    required_tags = sorted({clean(tag) for tag in config.get("required_tags", []) if clean(tag)})
    advisory_tags = sorted({clean(tag) for tag in config.get("advisory_tags", []) if clean(tag)})
    cases = [
        case
        for case in config.get("cases", [])
        if isinstance(case, dict)
        and case_selected(case, selected_case_keys=selected_case_keys, include_tags=include_tags)
    ]
    out_dir.mkdir(parents=True, exist_ok=True)
    case_reports = [
        evaluate_case(case, matrix_out_dir=out_dir, run_command=run_commands)
        for case in cases
    ]
    seen_tags = sorted({
        tag
        for report in case_reports
        if report["passed"]
        for tag in report.get("tags", [])
    })
    failures = [
        {"case": report["key"], "kind": "case_failed", "detail": failure}
        for report in case_reports
        for failure in report["failures"]
    ]
    for tag in required_tags:
        if tag not in seen_tags:
            failures.append({
                "case": None,
                "kind": "missing_required_tag",
                "detail": f"no passing case covered required tag {tag}",
            })
    advisory_gaps = [
        {
            "kind": "missing_advisory_tag",
            "tag": tag,
            "detail": advisory_tag_detail(config, tag),
        }
        for tag in advisory_tags
        if tag not in seen_tags
    ]
    if require_advisory_tags:
        failures.extend({
            "case": None,
            "kind": row["kind"],
            "detail": row["detail"],
        } for row in advisory_gaps)
    if not case_reports:
        failures.append({"case": None, "kind": "no_cases", "detail": "no matrix cases selected"})

    return {
        "matrix_key": matrix_key,
        "out_dir": str(out_dir),
        "run_commands": run_commands,
        "passes_matrix": not failures,
        "case_count": len(case_reports),
        "required_tags": required_tags,
        "advisory_tags": advisory_tags,
        "covered_tags": seen_tags,
        "advisory_gaps": advisory_gaps,
        "require_advisory_tags": require_advisory_tags,
        "cases": case_reports,
        "failures": failures,
    }


def advisory_tag_detail(config: dict[str, Any], tag: str) -> str:
    details = config.get("advisory_tag_details")
    if isinstance(details, dict):
        detail = clean(details.get(tag))
        if detail:
            return detail
    return f"no passing case covered advisory tag {tag}"


def evaluate_case(case: dict[str, Any], *, matrix_out_dir: Path, run_command: bool) -> dict[str, Any]:
    key = clean(case.get("key"))
    if not key:
        return {
            "key": "",
            "tags": [],
            "out_dir": "",
            "passed": False,
            "failures": ["case is missing key"],
        }
    tags = sorted({clean(tag) for tag in case.get("tags", []) if clean(tag)})
    case_out_dir = Path(expand_template(clean(case.get("out_dir")) or "{out_dir}/{case_key}", matrix_out_dir, key))
    case_out_dir.mkdir(parents=True, exist_ok=True)
    failures: list[str] = []
    command_status: dict[str, Any] | None = None
    if run_command:
        command_status = run_case_command(case, case_out_dir, matrix_out_dir, key)
        if command_status["returncode"] != 0:
            failures.append(
                f"command failed with status {command_status['returncode']} log={command_status['log']}"
            )
            return case_report(case, key, tags, case_out_dir, command_status, failures, {})

    metrics: dict[str, Any] = {}
    validate_contract_reports(case, case_out_dir, failures, metrics)
    validate_promotion_audit_reports(case, case_out_dir, failures, metrics)
    validate_eval_reports(case, case_out_dir, failures, metrics)
    validate_comparison_reports(case, case_out_dir, failures, metrics)
    validate_forbidden_terms(case, case_out_dir, failures, metrics)
    return case_report(case, key, tags, case_out_dir, command_status, failures, metrics)


def case_report(
    case: dict[str, Any],
    key: str,
    tags: list[str],
    case_out_dir: Path,
    command_status: dict[str, Any] | None,
    failures: list[str],
    metrics: dict[str, Any],
) -> dict[str, Any]:
    return {
        "key": key,
        "description": clean(case.get("description")),
        "tags": tags,
        "out_dir": str(case_out_dir),
        "passed": not failures,
        "command": command_status,
        "metrics": metrics,
        "failures": failures,
    }


def case_selected(case: dict[str, Any], *, selected_case_keys: set[str], include_tags: set[str]) -> bool:
    key = clean(case.get("key"))
    tags = {clean(tag) for tag in case.get("tags", []) if clean(tag)}
    if selected_case_keys and key not in selected_case_keys:
        return False
    if include_tags and not tags.intersection(include_tags):
        return False
    return True


def run_case_command(case: dict[str, Any], case_out_dir: Path, matrix_out_dir: Path, key: str) -> dict[str, Any]:
    command = case.get("command")
    if not isinstance(command, list) or not command or not all(isinstance(part, str) for part in command):
        return {"returncode": 2, "log": "", "command": command, "detail": "invalid command"}
    expanded_command = [expand_template(part, matrix_out_dir, key) for part in command]
    env = os.environ.copy()
    env["OUT_DIR"] = str(case_out_dir)
    for env_key, env_value in (case.get("env") or {}).items():
        env[str(env_key)] = expand_template(str(env_value), matrix_out_dir, key)
    log_path = case_out_dir / "matrix_command.log"
    with log_path.open("w", encoding="utf-8") as log:
        completed = subprocess.run(
            expanded_command,
            cwd=ROOT_DIR,
            env=env,
            stdout=log,
            stderr=subprocess.STDOUT,
            check=False,
            text=True,
        )
    return {
        "returncode": completed.returncode,
        "log": str(log_path),
        "command": expanded_command,
    }


def validate_contract_reports(case: dict[str, Any], case_out_dir: Path, failures: list[str], metrics: dict[str, Any]) -> None:
    reports = normalize_report_specs(case.get("contract_reports"))
    passed = 0
    for spec in reports:
        path = case_out_dir / spec["path"]
        data = read_report(path, failures)
        if not data:
            continue
        if not data.get("passes_contract"):
            failures.append(f"contract report failed: {path}")
        else:
            passed += 1
    if reports:
        metrics["contract_report_count"] = len(reports)
        metrics["contract_pass_count"] = passed


def validate_promotion_audit_reports(case: dict[str, Any], case_out_dir: Path, failures: list[str], metrics: dict[str, Any]) -> None:
    reports = normalize_report_specs(case.get("promotion_audit_reports"))
    passed = 0
    promotable_associations = 0
    blocked_associations = 0
    for spec in reports:
        path = case_out_dir / spec["path"]
        data = read_report(path, failures)
        if not data:
            continue
        if not data.get("passes_promotion_audit"):
            failures.append(f"promotion audit failed: {path}")
        if data.get("promotable_association_count", 0) < spec.get("min_promotable_association_count", 0):
            failures.append(f"promotion audit has too few promotable associations: {path}")
        max_blocked = spec.get("max_blocked_association_count")
        if max_blocked is not None and data.get("blocked_association_count", 0) > max_blocked:
            failures.append(f"promotion audit has too many blocked associations: {path}")
        if data.get("passes_promotion_audit"):
            passed += 1
        promotable_associations += int(data.get("promotable_association_count") or 0)
        blocked_associations += int(data.get("blocked_association_count") or 0)
    if reports:
        metrics["promotion_audit_report_count"] = len(reports)
        metrics["promotion_audit_pass_count"] = passed
        metrics["promotable_association_count"] = promotable_associations
        metrics["blocked_association_count"] = blocked_associations


def validate_eval_reports(case: dict[str, Any], case_out_dir: Path, failures: list[str], metrics: dict[str, Any]) -> None:
    reports = normalize_report_specs(case.get("eval_reports"))
    passed = 0
    pass_total = 0
    question_total = 0
    for spec in reports:
        path = case_out_dir / spec["path"]
        data = read_report(path, failures)
        if not data:
            continue
        golden = data.get("golden_eval") if isinstance(data.get("golden_eval"), dict) else {}
        pass_count = int(golden.get("pass_count") or 0)
        question_count = int(golden.get("question_count") or 0)
        pass_total += pass_count
        question_total += question_count
        require_passes_eval = bool(spec.get("require_passes_eval", True))
        if require_passes_eval and not data.get("passes_eval"):
            failures.append(f"eval report failed: {path}")
        if spec.get("require_passes_smoke_eval") and not data.get("passes_smoke_eval"):
            failures.append(f"smoke eval failed: {path}")
        if pass_count < spec.get("min_pass_count", 0):
            failures.append(f"eval pass count below minimum for {path}: {pass_count}")
        expected_question_count = spec.get("question_count")
        if expected_question_count is not None and question_count != expected_question_count:
            failures.append(f"eval question count mismatch for {path}: {question_count} != {expected_question_count}")
        if data.get("passes_eval"):
            passed += 1
    if reports:
        metrics["eval_report_count"] = len(reports)
        metrics["eval_pass_count"] = passed
        metrics["golden_pass_total"] = pass_total
        metrics["golden_question_total"] = question_total


def validate_comparison_reports(case: dict[str, Any], case_out_dir: Path, failures: list[str], metrics: dict[str, Any]) -> None:
    reports = normalize_report_specs(case.get("comparison_reports"))
    passed = 0
    gate_count = 0
    for spec in reports:
        path = case_out_dir / spec["path"]
        data = read_report(path, failures)
        if not data:
            continue
        gates = data.get("promotion_gates") if isinstance(data.get("promotion_gates"), list) else []
        gate_count += len(gates)
        if not data.get("passes_promotion_gates"):
            failures.append(f"promotion comparison failed: {path}")
        else:
            passed += 1
    if reports:
        metrics["comparison_report_count"] = len(reports)
        metrics["comparison_pass_count"] = passed
        metrics["promotion_gate_count"] = gate_count


def validate_forbidden_terms(case: dict[str, Any], case_out_dir: Path, failures: list[str], metrics: dict[str, Any]) -> None:
    terms = [str(term) for term in case.get("forbidden_terms", []) if str(term)]
    if not terms:
        return
    scan_patterns = [str(pattern) for pattern in case.get("scan_paths", []) if str(pattern)]
    if not scan_patterns:
        scan_patterns = ["*.json", "*.md", "*.txt"]
    scanned = 0
    hits: list[dict[str, str]] = []
    for pattern in scan_patterns:
        for raw_path in sorted(glob.glob(str(case_out_dir / pattern))):
            path = Path(raw_path)
            if not path.is_file():
                continue
            scanned += 1
            text = path.read_text(encoding="utf-8", errors="ignore")
            for term in terms:
                if term in text:
                    hits.append({"path": str(path), "term": term})
    if not scanned:
        failures.append(f"forbidden-term scan matched no files in {case_out_dir}")
    if hits:
        failures.append("forbidden terms found: " + ", ".join(f"{hit['term']}@{hit['path']}" for hit in hits[:12]))
    metrics["forbidden_term_scan_file_count"] = scanned
    metrics["forbidden_term_hit_count"] = len(hits)


def normalize_report_specs(value: Any) -> list[dict[str, Any]]:
    if value is None:
        return []
    if not isinstance(value, list):
        value = [value]
    out: list[dict[str, Any]] = []
    for row in value:
        if isinstance(row, str):
            out.append({"path": row})
        elif isinstance(row, dict) and clean(row.get("path")):
            out.append(dict(row))
    return out


def read_report(path: Path, failures: list[str]) -> dict[str, Any]:
    if not path.exists():
        failures.append(f"missing report: {path}")
        return {}
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        failures.append(f"invalid JSON report {path}: {exc}")
        return {}
    if not isinstance(data, dict):
        failures.append(f"report is not a JSON object: {path}")
        return {}
    return data


def load_json(path: Path) -> dict[str, Any]:
    data = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(data, dict):
        raise SystemExit(f"{path} must contain a JSON object")
    return data


def expand_template(value: str, matrix_out_dir: Path, case_key: str) -> str:
    return os.path.expandvars(
        value.replace("{out_dir}", str(matrix_out_dir))
        .replace("{root_dir}", str(ROOT_DIR))
        .replace("{case_key}", case_key)
    )


def clean(value: Any) -> str:
    return str(value or "").strip()


def print_matrix_summary(report: dict[str, Any]) -> None:
    print(
        "bounded_graph_promotion_matrix",
        report.get("matrix_key"),
        "passes_matrix=" + str(report.get("passes_matrix")),
        "cases=" + str(report.get("case_count")),
    )
    for gap in report.get("advisory_gaps", []):
        print(
            "bounded_graph_promotion_matrix",
            "advisory_gap",
            gap.get("tag"),
            gap.get("detail"),
        )
    for case in report.get("cases", []):
        metrics = case.get("metrics") or {}
        print(
            "bounded_graph_promotion_matrix",
            case.get("key"),
            "passed=" + str(case.get("passed")),
            "eval=" + str(metrics.get("golden_pass_total")) + "/" + str(metrics.get("golden_question_total")),
            "promotion_audits=" + str(metrics.get("promotion_audit_pass_count")) + "/" + str(metrics.get("promotion_audit_report_count")),
            "contracts=" + str(metrics.get("contract_pass_count")) + "/" + str(metrics.get("contract_report_count")),
        )


def format_failures(report: dict[str, Any]) -> str:
    failures = report.get("failures") or []
    if not failures:
        return "bounded graph promotion matrix failed"
    lines = ["bounded graph promotion matrix failed:"]
    for failure in failures[:20]:
        case = failure.get("case") or "matrix"
        lines.append(f"- {case}: {failure.get('detail')}")
    return "\n".join(lines)


if __name__ == "__main__":
    main()
