# Real Connector Source-Scope Runtime Probe

This deterministic pack narrows the remaining source-scope blocker without
overstating real coverage.

Current real Flink/Jira/GitHub captures contain real `fresh + partial_scope`
source-scope rows, but not a genuinely real stale-window or not-attempted
source-scope row. This pack therefore copies the real
`ontology.source-scope-claimable-20260624.db` database and derives two runtime
probes:

- stale exact-scope Jira coverage for `ticket:jira:FLINK-36332`;
- never-attempted unknown Jira coverage for the same real ticket/relationship
  slice.

It verifies that:

- stale source scope gates absence claims with `source_scope_not_fresh`;
- positive Jira remote-link relationships remain visible and claimable;
- never-attempted source scope is not prompt-contract ready because it has no
  coverage window;
- current real source-scope captures still lack a real stale/not-attempted row.

Scope note: this is a derived-real runtime proof, not real source-scope
evidence. The product-safe blocker stays open until a connector capture contains
real stale-window or source-not-attempted state.
