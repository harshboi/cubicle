# Production-Like Slack ACL Ingestion

This pack is a replayable OpenGraph connector fixture shaped like a Slack
workspace/channel sync. It is intentionally small and single-source so ACL
provenance can be audited without DB co-location ambiguity.

It proves the hard ACL inventory gate can see source-backed non-public product
rows on candidate graph relationships:

- `SourceConnection(connector_kind=slack_api)`
- `SourceScope(channel=C-incident)`
- private `slack_message`
- private `contains_message` relationship
- matching `source_system=slack` and `source_instance=slack.example.test/workspace-a`

This is production-like replay evidence for the PoC. It is not a live private
Slack workspace capture.
