#!/usr/bin/env python3
"""Render and evaluate a cited brief from a bounded graph context.

This CLI is the AI-first PoC surface. It intentionally accepts only the
generic ``boundedGraphContext`` contract and leaves WorkProgram, analytics DB,
Ent DB, and persistence paths in ``cubicle_graph_brief.py``.
"""

from __future__ import annotations

import argparse
import json
from pathlib import Path

import bounded_graph_contract
import cubicle_graph_brief as graph_brief


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Build a generic graph-safety brief from a boundedGraphContext JSON payload."
    )
    parser.add_argument("--bounded-graph-context-json", type=Path, required=True)
    parser.add_argument("--context-json", type=Path, required=True)
    parser.add_argument("--brief-md", type=Path, required=True)
    parser.add_argument("--typed-row-baseline-md", type=Path)
    parser.add_argument("--generic-baseline-md", type=Path)
    parser.add_argument("--prompt-md", type=Path)
    parser.add_argument("--prompt-mode", choices=["generic"], default="generic")
    parser.add_argument("--llm-command")
    parser.add_argument("--mlx-model")
    parser.add_argument("--mlx-python", default="/Users/harsh/.venv-vllm-metal/bin/python")
    parser.add_argument("--llm-max-tokens", type=int, default=8192)
    parser.add_argument("--llm-brief-md", type=Path)
    parser.add_argument("--repaired-brief-md", type=Path)
    parser.add_argument("--evaluation-json", type=Path)
    parser.add_argument("--golden-json", type=Path)
    parser.add_argument("--llm-timeout-seconds", type=int, default=120)
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> None:
    args = parse_args(argv)
    validate_bounded_graph_input(args.bounded_graph_context_json)
    context = graph_brief.build_context_bundle_from_bounded_graph_context_json(args.bounded_graph_context_json)

    args.context_json.parent.mkdir(parents=True, exist_ok=True)
    args.brief_md.parent.mkdir(parents=True, exist_ok=True)
    args.context_json.write_text(json.dumps(context, indent=2, sort_keys=True), encoding="utf-8")
    args.brief_md.write_text(graph_brief.render_generic_graph_baseline(context), encoding="utf-8")

    if args.typed_row_baseline_md:
        args.typed_row_baseline_md.parent.mkdir(parents=True, exist_ok=True)
        args.typed_row_baseline_md.write_text(graph_brief.render_typed_row_baseline(context), encoding="utf-8")
    if args.generic_baseline_md:
        args.generic_baseline_md.parent.mkdir(parents=True, exist_ok=True)
        args.generic_baseline_md.write_text(graph_brief.render_generic_graph_baseline(context), encoding="utf-8")

    prompt = graph_brief.render_prompt(context, mode="generic")
    if args.prompt_md:
        args.prompt_md.parent.mkdir(parents=True, exist_ok=True)
        args.prompt_md.write_text(prompt, encoding="utf-8")

    llm_command = args.llm_command
    if args.mlx_model:
        if llm_command:
            raise SystemExit("--mlx-model and --llm-command are mutually exclusive")
        llm_command = graph_brief.mlx_lm_command(args.mlx_python, args.mlx_model, args.llm_max_tokens)
    if llm_command:
        if not args.llm_brief_md:
            raise SystemExit("--llm-brief-md is required when --llm-command or --mlx-model is provided")
        args.llm_brief_md.parent.mkdir(parents=True, exist_ok=True)
        args.llm_brief_md.write_text(
            graph_brief.run_llm_command(llm_command, prompt, args.llm_timeout_seconds),
            encoding="utf-8",
        )

    if args.repaired_brief_md or args.evaluation_json or args.golden_json:
        if not args.llm_brief_md:
            raise SystemExit(
                "--llm-brief-md is required when --repaired-brief-md, --evaluation-json, or --golden-json is provided"
            )
        raw_answer_text = args.llm_brief_md.read_text(encoding="utf-8")
        golden_spec = json.loads(args.golden_json.read_text(encoding="utf-8")) if args.golden_json else None
        raw_evaluation = graph_brief.evaluate_brief_for_gates(context, raw_answer_text, golden_spec)
        answer_text = raw_answer_text
        if args.repaired_brief_md:
            answer_text = graph_brief.repair_llm_brief(context, answer_text)
            args.repaired_brief_md.parent.mkdir(parents=True, exist_ok=True)
            args.repaired_brief_md.write_text(answer_text, encoding="utf-8")
        evaluation = graph_brief.evaluate_brief_for_gates(context, answer_text, golden_spec)
        evaluation["prompt_mode"] = "generic"
        evaluation["repair_applied"] = bool(args.repaired_brief_md)
        evaluation["evaluated_answer_kind"] = "repaired" if args.repaired_brief_md else "raw"
        if args.repaired_brief_md:
            evaluation["repair_changed_answer"] = raw_answer_text != answer_text
            evaluation["raw_answer_eval"] = raw_evaluation
        if args.evaluation_json:
            args.evaluation_json.parent.mkdir(parents=True, exist_ok=True)
            args.evaluation_json.write_text(json.dumps(evaluation, indent=2, sort_keys=True), encoding="utf-8")


def validate_bounded_graph_input(path: Path) -> None:
    report = bounded_graph_contract.validate_bounded_graph_context_file(path)
    if report["error_count"]:
        raise SystemExit(bounded_graph_contract.format_contract_errors(report))


if __name__ == "__main__":
    main()
