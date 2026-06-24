# Real PyPI Source Scope Stale

This pack replays the public PyPI `requests` OpenGraph connector snapshot with
explicit `pypi_json_api` source-scope provenance, then marks the current
`SourceScopeState` stale.

It proves the inventory hard gate can see a production-like public connector
candidate whose graph rows reference stale source-scope state:

- `SourceConnection(connector_kind=pypi_json_api)`
- `SourceScope(project=requests)`
- `SourceScopeState(freshness_state=stale, coverage_mode=partial_scope)`
- `pypi_project -> pypi_release`
- `pypi_release -> pypi_distribution`
- `pypi_project -> package_contact`

This is a positive-fact traversal/provenance case. It does not prove private
workspace ACL translation, and OpenGraph absence coverage remains sparse until
there is an explicit OpenGraph source-scope coverage policy.
