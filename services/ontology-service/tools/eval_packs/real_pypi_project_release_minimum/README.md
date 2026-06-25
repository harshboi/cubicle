# Real PyPI Project Release Minimum

This pack is a replayable OpenGraph connector snapshot captured from the public
PyPI JSON endpoint for `requests` on 2026-06-24.

It proves a second real, non-GitHub source domain can use the same generic
bounded graph path with explicit source-scope provenance:

- `pypi_project` -> `pypi_release` with `has_release`
- `pypi_release` -> `pypi_distribution` with `has_distribution`
- `pypi_project` -> `package_contact` with `has_contact`
- `SourceConnection(connector_kind=pypi_json_api)` -> `SourceScope(project=requests)` -> fresh `partial_scope` `SourceScopeState`

The fixture is intentionally sparse. Missing releases, files, maintainers, or
neighboring projects are unknown, not absent. This public PyPI pack does not
prove non-public ACL translation or product-safe absence claims.
