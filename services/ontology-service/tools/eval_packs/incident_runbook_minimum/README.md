# incident-runbook-minimum

This pack is a non-ticket/PR falsifier for the generic bounded graph PoC.

It uses the memory-store fixture `customer-incident-runbook`, whose primary path
is:

```text
customer_account -> incident -> slack_message
                       \-> runbook_document
```

The fixture also contains an unrelated finance incident behind a shared Slack
channel. The eval exports only the incident/runbook relationship families, then
injects a visible disconnected finance distractor into the saved context to
verify the generic brief renderer does not select unrelated rows.

Run:

```text
sh tools/eval_packs/incident_runbook_minimum/run_eval.sh
```
