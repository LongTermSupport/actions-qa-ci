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
  and a green fixture. The project's own actionlint configuration is deliberately not applied.
- ShellCheck (v0.11.0) on the `bash` and `sh` scripts in `run:` steps, through actionlint:
  findings are `actionsqaci.shellcheck.SC<nnnn>`, all codes sharing one page, at `warning` and
  above by default. The binary is pinned by sha256 in a manifest, verified before every run, and
  never taken from `PATH`; `actions-qa-dev fetch <dir>` downloads it for development.
- The `rule <id> <path>` harness: runs one rule on one file and prints only that rule's
  findings, exiting 1 when it fires.
- The `text`, `agent` and `json` output formats, and the finding model with its identifier
  scheme.
