# Real Connector Negative / Partial Pack

This deterministic pack turns real Flink/Jira/GitHub connector DBs into
negative evidence for product-safe bounded graph behavior.

It checks three current real cases:

- `ticket:jira:FLINK-32695` in `ontology.ai-tpm-1000-20260622.db`: six visible
  ticket-to-PR relationships whose PR endpoints are partial, so objects and
  relationships must remain non-claimable.
- `pull-request:github:apache/flink-kubernetes-operator#998` in
  `ontology.ai-tpm-1000-open-auth-hydrated-20260622.db`: a real source
  `401`/non-200 and missing-snapshot pair on PR files must force limited
  coverage without leaking raw issue body text or source URLs into bounded
  context. The same slice keeps two `approver` edges and one `implemented_by`
  edge claimable while gating one `author` edge whose evidence locator is not
  authoritative for author presence.
- `ticket:jira:FLINK-36332` in `ontology.source-scope-claimable-20260624.db`:
  positive Jira remote-link relationships stay claimable, but fresh
  `partial_scope` source state still blocks absence claims with
  `source_scope_not_exact`.

The pack does not yet prove a real stale-window source-scope state; current
available real DBs contain `fresh + partial_scope` source states, not stale
source-scope rows.
