# Real Connector ACL Probe

This deterministic pack proves the bounded graph runtime can enforce
principal-scoped visibility on connector-derived rows before traversal and
fanout.

The pack copies the real Flink/Jira/GitHub SQLite capture at:

`/Users/harsh/workspace/data/flink-pr-jira-1000-plus-500-jira-2026-06-22/ontology.ai-tpm-1000-open-auth-hydrated-20260622.db`

Then it marks one existing `FLINK-36332 -> PR` relationship and endpoint as
`private` with a higher rank than a still-public sibling. It verifies:

- a public-only export skips the private high-rank edge and returns the public
  fallback;
- a private-allowed export returns the private high-rank edge;
- private endpoint/edge text does not appear in the public context;
- the private-allowed context still treats restricted facts as non-promotable
  product claims.

Scope note: this is a derived-real connector runtime proof, not proof that the
current Jira/GitHub loaders ingest real source ACLs. The product-safe readiness
blocker for real connector ACL translation should remain open until a real
connector maps source ACLs into `visibility` / `acl_state` without mutation.
