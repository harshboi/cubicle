# Operating Brief

## Confirmed Facts

- GitHub PR 42 is present as a typed graph row. [work_program_items:work-program-item:github-pr-42]
- Support ticket SUP-101 is present without a linked PR requirement. [work_program_items:work-program-item:support-ticket-101]
- Ticket TCK-7 links to GitHub PR 42 through a typed relationship. [ticket_pull_requests:ticket-pr:TCK-7:github-pr-42]

## Validation Leads

- Forecast output is risk triage only. [analytics:tpm_forecast_summary]
- Missing reviewer evidence is an evidence need. [work_program_evidence_needs:evidence-need:reviewer-proof]
- Derived dependency edges are topology context, not product truth. [work_dependency_edges:work-dependency-edge:derived-42]

## What Not To Claim

- Sparse coverage blocks absence claims. [source_coverage:workstream:support]
- GitHub 403 and 429 results are coverage evidence only. [source_coverage:workstream:github-auth-limited]
- Generated graph briefs are generated evidence, not source truth. [guardrail:mixedctx]
- runKey bounds packet rows while graph rows are latest scoped rows. [context:mixedctx]
