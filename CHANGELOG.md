# Changelog

## Unreleased

### Added

- The repository floor: the Go module, the `actions-qa` command with `version [--json]` and
  `help`, the documented exit codes, the `dbf.json` conformance declaration and a continuous
  integration workflow.
- The `rules [--json]` and `rule-doc <id>` commands, the rule registry, and the generated
  identifier index under `docs/rules/` with a self-audit that every rule has a complete page.
- The actionlint backend, compiled in as a library: fifteen rules,
  `actionsqaci.actionlint.<kind>`, each with a page stating the correct construction and a red
  and a green fixture. The project's own actionlint configuration is deliberately not applied.
- ShellCheck on the `bash` and `sh` scripts in `run:` steps, through actionlint:
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
- `check` runs the actionlint lane on each file in a process of its own, with a two-minute time
  limit and a cap on its output. A workflow that crashes actionlint's parser or makes it run
  without end is exit 4, naming the lane and the file, and is never reported as clean; a lane
  that fails on its own account is exit 3. The `rule` harness still runs actionlint in its own
  process, which `dbf.json` declares.
- The zizmor backend: a `zizmor` lane that audits workflows, composite actions and the
  Dependabot file, reporting each finding as `actionsqaci.zizmor.<audit>` with a page for every
  one of zizmor's audits and a red and a green fixture for each that runs. zizmor is the pinned
  static executable taken from its PyPI wheel (verified by sha256, never taken from `PATH`),
  run offline at its regular persona with no configuration, no honoured ignore comment and none
  of the caller's environment; a file it cannot parse is a fault and a zizmor ended by a signal or
  the time limit is exit 4. The online audits, `forbidden-uses` and `insecure-url-scheme` have an
  identifier and a page but are not run yet; `dbf.json` declares it. The pedantic and auditor
  audits the regular persona leaves out are reported by `check` where the project record's
  `calibrations.enable` names them, and `actions-qa rules` shows each such audit with an `enabled:`
  line. The `rule` harness shows what any audit finds at zizmor's widest persona.
  A hazard both tools report has one owner where the owner reports everything the other would:
  actionlint's `credentials` kind is dropped, so `actionsqaci.zizmor.hardcoded-container-credentials`
  is the only identifier a finding of it carries, and `rules` states what the owner absorbs.
  actionlint's "potentially untrusted" `expression` messages are kept, because zizmor's regular
  persona does not report a `fromJSON` of an untrusted input; `dbf.json` declares the overlap.
  A run in which the zizmor lane analyses a file names, on stderr, each audit it does not run and
  why, so a clean result is not read as covering the network audits.
- Two guards against suppression outside the project record. `actionsqaci.inlineSuppression`
  refuses a comment that tells a tool to leave a finding out (`# zizmor: ignore`,
  `# ratchet:exclude`, `# shellcheck disable=`) in a workflow or action, which closes the declared
  gap that a ShellCheck directive in a `run:` script went unreported.
  `actionsqaci.nativeSuppressionConfig` refuses a zizmor configuration file, an actionlint
  configuration with an `ignore` key and a pinact configuration with `ignore_actions`; `check`
  now looks for those files by default. The guards read files and do not lint them.
- `actionsqaci.templateDrift` and the record's `templates` section (a `source` and its `copies`):
  a copy that differs from its source in any byte is a finding at the copy, naming the first
  differing line, and a source or copy that is missing, not a plain file or over 1 MiB is a
  finding at the record, so renaming a copy cannot silence the check. Files are opened without
  blocking and without following a link, so a pipe in place of a copy is reported and never read.
  Whole-file comparison only; the design's `identical-except` mode is declared as a gap.
- `actionsqaci.requiredChecks` and the record's `requiredChecks` section (a `context` and an
  optional `branch`): a required status check that no job of a workflow triggered on
  `pull_request` produces is a finding at its entry in the record, saying why when a job nearly
  does (the workflow is not a pull request one, the job name holds an expression, or the job
  runs a matrix). A job's check run is its `name:` or else its id; a job that calls a reusable
  workflow produces contexts under `<name> / `. A workflow that cannot be read is a finding at
  that workflow. Comparing with the repository's real branch protection is declared as a gap.
- `actionsqaci.lintCoverage` and the record's `analyse.unanalysed` section: a tracked file that
  is workflow-shaped (top-level `on` and `jobs`) or action-shaped (`action.yml` with `runs`) and is
  neither in the default set nor declared with a justification is a finding at the file, and a
  declaration that excuses no file is a finding at its entry. The tracked files come from the
  repository's own index (versions 2 to 4, checked against its checksum; a split or sparse index is
  refused), so no `git` is run. `check` runs this lane once for the project, with or without a
  record.
- `check --online`: compares the record's `requiredChecks` with the branch protection and
  rulesets the repository really has (the lane `requiredChecksOnline`, reporting as
  `actionsqaci.requiredChecks`). A context the record requires and the protection does not, and
  one the protection requires and the record does not list, are findings. The token comes from
  `GITHUB_TOKEN` or `GH_TOKEN`, the repository from `GITHUB_REPOSITORY` and the API from
  `GITHUB_API_URL` (https only; the token is sent nowhere else, no redirect is followed, and it is
  never printed). A protection that cannot be read is exit 4. Without `--online` nothing reaches
  the network, and a run with required checks in its record says on standard error that the
  protection was not compared.
- `actions-qa rules --write-agent-summary <file>`: writes the active defences, and what the
  project record narrows, into a generated region of `CLAUDE.md` or `AGENTS.md` (between
  `<!-- actions-qa-ci:begin -->` and `<!-- actions-qa-ci:end -->`), keeping the rest of the file
  byte for byte. A file whose markers do not form one region is refused and left alone.
- `actionsqaci.agentSummaryCurrent`: the generated region of `CLAUDE.md` or `AGENTS.md` at the
  repository root has to be the one `rules --write-agent-summary` would write now (the defences
  of the build, and what the project record narrows); a stale region is a finding at its begin
  marker, markers that do not form one region a finding at the file. A file with no region is
  left alone. `check` runs the lane once for the project, with or without a record.
- The files `--sarif` and `--write-agent-summary` write are now replaced whole (written beside the
  file and moved into place, permissions kept) instead of truncated and rewritten, so a write
  that fails leaves the old file, and another name hard-linked to it keeps the old content.
- The record's `analyse.include` section: patterns of tracked `.yml` or `.yaml` files outside the
  default set that the project wants analysed (a shipped template, say). An included workflow is
  linted by actionlint and zizmor and guarded against inline suppression; an included
  `action.yml` is audited by zizmor and guarded against inline suppression. Such a file can be
  named on the command line like any other. A pattern that
  names no such file is an `actionsqaci.lintCoverage` finding at its entry; a file asked for that
  is a link, is missing or has an unprintable name is exit 4.
- The `github` output format (`--format github`, or `ACTIONS_QA_FORMAT=github`): workflow
  annotations for the findings, ten of each level at most, then the full list as the `agent`
  format prints it. Every format now prints a path that starts `::` or `##[` quoted, so a file
  name cannot be read by the Actions runner as a workflow command.
- `check --sarif <file>`: writes the findings as one SARIF 2.1.0 log with a single run (driver
  `actions-qa-ci`, actionlint, ShellCheck and zizmor as extensions with their versions, the upstream
  identifier in each result's properties). A finding covered by an exception stays in the log with
  an external suppression. The file is never written through a symbolic link or to a pipe, and a
  link in the repository cannot send it outside; a log that is asked for and cannot be written is
  exit 3. The `github` format says "1 warning annotation was left out" in the singular.
- The `rule <id> <path>` harness: runs one rule on one file and prints only that rule's
  findings, exiting 1 when it fires.
- The `text`, `agent` and `json` output formats, and the finding model with its identifier
  scheme.
