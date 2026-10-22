# Changelog

## Unreleased

### Added

- The repository floor: the Go module, the `actions-qa` command with `version [--json]` and
  `help`, the documented exit codes, the `dbf.json` conformance declaration and a continuous
  integration workflow.
- The `rules [--json]` and `rule-doc <id>` commands, the rule registry, and the generated
  identifier index under `docs/rules/` with a self-audit that every rule has a complete page.
- The actionlint backend (v1.7.12), compiled in as a library: sixteen rules,
  `actionsqaci.actionlint.<kind>`, each with a page stating the correct construction and a red
  and a green fixture. The project's own actionlint configuration is deliberately not read.
- The `rule <id> <path>` harness: runs one rule on one file and prints only that rule's
  findings, exiting 1 when it fires.
- The `text`, `agent` and `json` output formats, and the finding model with its identifier
  scheme.
