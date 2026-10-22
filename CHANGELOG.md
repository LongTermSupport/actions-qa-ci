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
- The project record, `.github/actions-qa-ci.yml`: a strict loader for its `version` and
  `exceptions`, and `actionsqaci.unjustifiedException`, which refuses an exception whose hazard,
  scope or decision is missing, generic, too short or points elsewhere. The record is read and
  checked; applying an exception to a finding comes with the `check` command.
- The `check` command: validates the project record, then runs the actionlint (with ShellCheck)
  and first-party lanes over the workflows, actions, Dependabot file and record the project has,
  or over the files named. `-t` picks lanes, `--format` and `ACTIONS_QA_FORMAT` pick the output.
  A failing lane stops the run with exit 3 and no findings; a file that cannot be read as a plain
  file is exit 4; a directory that is not the repository root is refused. A discovered file no
  lane analyses is named on standard error.
- Exceptions are applied. `check` marks a finding covered by an exception of the project record
  (same identifier and path, and the job and step if named) as excepted: counted, not listed. In
  a full run an exception that covers nothing is `actionsqaci.staleException`, at its line in the
  record. `rules` shows beside a defence the record turns off everywhere: a `narrowed:` line in
  the text format and a `narrowedBy` array on the entry in `--json`, absent when nothing narrows
  it; it reads the record in the working directory and refuses an invalid one. The two checks on
  the record itself cannot be excepted.
- The `rule <id> <path>` harness: runs one rule on one file and prints only that rule's
  findings, exiting 1 when it fires.
- The `text`, `agent` and `json` output formats, and the finding model with its identifier
  scheme.
