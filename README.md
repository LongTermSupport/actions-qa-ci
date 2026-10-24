# actions-qa-ci

actions-qa-ci is the [Defence Before Fix](https://defence-before-fix.github.io/) toolchain for
GitHub Actions code: workflows, composite actions and the shell they run. It is one static
program, `actions-qa`, that bundles its rule backends and gives every finding a stable
identifier and a page stating the correct construction.

This repository is being built in phases. What exists today is the core: the Go module, the
conformance declaration, the finding model and identifier scheme, the rule registry, the output
formats, the `version`, `rules`, `rule-doc`, `rule` and `check` commands, and the repository's own
CI. The first backend is compiled in: actionlint, as sixteen rules, with ShellCheck running on the
shell in `run:` steps. The remaining backends and the first-party checks arrive in later phases.

## Requirements

- Go, to build from source. A released bundle needs nothing on the host.

## Use

```bash
go run ./cmd/actions-qa version          # the specification versions this build is audited against
go run ./cmd/actions-qa version --json   # the same, as the conformance declaration
go run ./cmd/actions-qa rules            # every active rule, without running it (--json for a list)
go run ./cmd/actions-qa rule-doc <id>    # the page for a rule identifier, offline
go run ./cmd/actions-qa check            # every lane over the project's files (-t lane[,lane], --format, path…)
go run ./cmd/actions-qa rule <id> <path> # one rule on one file, only its findings (--format text|agent|json|github)
go run ./cmd/actions-qa help
```

Every rule has an identifier, and every identifier has a page under `docs/rules/` that states
the correct construction. `docs/rules/README.md` is the identifier index; it is generated from
the registry and a test fails when it is stale (`go test . -run TestTheIdentifierIndexIsCurrent
-update` regenerates it). A test also fails when a rule has no page, when a page lacks one of
its five required sections (in order, outside code fences, none empty), or when a page has no
rule.

The project's own actionlint configuration (`.github/actionlint.yaml`) is never applied: an ignore
there would suppress findings with no recorded justification, so the lane hands actionlint an empty
configuration. (actionlint still parses the file while locating the project, so a malformed one
fails the lane.) Suppression belongs in the project record, which this build reads and checks but does not
yet apply, so a finding is fixed. The lane must be run from the repository root, which holds `.git` and
`.github/workflows`, and refuses to run anywhere else: actionlint resolves local actions and
reusable workflows from the project it finds, and from a directory without both it would find a
different one.

The lane also runs ShellCheck (the release the manifest pins, which the SARIF log names) over the `bash` and `sh` scripts in `run:` steps, through
actionlint's integration, and reports each finding as `actionsqaci.shellcheck.SC<nnnn>`; all
codes share the page `docs/rules/actionsqaci.shellcheck.md`. Only `error` and `warning` findings
are reported by default. The ShellCheck binary is the one pinned in `internal/toolchain/manifest.json`
by sha256, looked for beside the program (or in the directory named by `ACTIONS_QA_CI_TOOLS_DIR`)
and verified before every run; one that is missing or differs fails the lane with exit 3, and a
ShellCheck on `PATH` is never used. In a development checkout, `go run ./cmd/actions-qa-dev fetch
bin` downloads it, and `ACTIONS_QA_CI_TOOLS_DIR=$PWD/bin` points the program at it. pyflakes
integration is not enabled.

### The zizmor lane

The `zizmor` lane runs zizmor (the release the manifest pins, a static musl executable taken out
of the PyPI wheel; the SARIF log names it) over one workflow, action or Dependabot file at a
time and reports each finding as `actionsqaci.zizmor.<audit>`, the audit's own kebab-case name,
with a page under `docs/rules/` for every audit. It is found and verified the way ShellCheck is
(beside the program, or in `ACTIONS_QA_CI_TOOLS_DIR`; never on `PATH`). Nothing of the project's or
the caller's is applied to it: no configuration file (`--no-config`), no ignore comment honoured
(`--no-ignores`; a finding a comment names is still reported), no network (`--offline`), none of
the caller's environment (only `NO_COLOR=1`) so that no token or `ZIZMOR_*` setting reaches it,
and `--strict-collection`, so a
file it cannot parse is a fault and not a clean run. Only the report decides: its exit codes are
turned off. A zizmor killed by a signal, or that runs past five minutes or writes more than 32 MiB,
leaves no verdict (exit 4); any other failure is exit 3.

The lane runs at zizmor's `regular` persona. The pedantic and auditor audits have identifiers
and pages, and `actions-qa rule <id> <path>` shows what each finds, but `check` does not report
them until the project record can enable them. The audits that need GitHub (`impostor-commit`,
`known-vulnerable-actions`, `ref-confusion`, `ref-version-mismatch`, `stale-action-refs`),
`forbidden-uses`, which does nothing without a policy, and `insecure-url-scheme`, which reads only
a pre-commit configuration, are held: they have an identifier and a page, `rule` says why it does
not run them, `check` names each of them, with its reason, on stderr whenever the zizmor lane is
among those it runs (`actions-qa: the zizmor lane does not run <identifier>: <reason>`, so a
clean result is not read as covering them), and `dbf.json` declares the gap. A finding of an audit this build has no
identifier for fails the lane, so a new zizmor audit cannot be passed over unseen.

A hazard both tools report has one owner. actionlint's `credentials` kind and the "potentially
untrusted" messages of its `expression` kind are dropped by the lane, because zizmor's
`hardcoded-container-credentials` and `template-injection` own those hazards; a finding of either
carries only the zizmor identifier, and so does the exception that records it. The type errors of
the `expression` kind stay actionlint's. `actions-qa rules` states what each owner absorbs.

### The check command

`actions-qa check` validates the project record, then runs the lanes (`actionlint`, which carries
ShellCheck's findings; `zizmor`, which audits workflows, actions and the Dependabot file;
`firstParty`, which checks the record; and two guards against suppression,
`inlineSuppression` and `nativeSuppressionConfig`, described below) over the files the project
has and prints every finding. It must be run from the repository root, which holds `.git`: from
anywhere else it would find nothing and read as clean, so it refuses.

- **Files.** By default: the workflows directly in `.github/workflows`, every `action.yml` or
  `action.yaml` under `.github/actions` and at the root, `.github/dependabot.yml` or
  `.github/dependabot.yaml`, the project record, and the configuration files of the bundled
  tools (`zizmor.yml`, `.github/actionlint.yaml`, `.pinact.yaml` and their other spellings, see
  `actionsqaci.nativeSuppressionConfig`). A lane analyses the files it covers; actionlint
  covers the workflows, zizmor the workflows, the actions and the Dependabot file, and the
  first-party lane the record. A file of the default set that no lane of the build lints is
  named on standard error, so a clean result is not read as covering it: the guards read the
  workflows and actions for suppression comments but do not lint them. Naming files on the command line analyses only those, and
  a file no chosen lane covers is refused rather than passed over. A symbolic link or other
  non-file where the default set looks (a directory named like a workflow or an action, at any depth) stops the run with exit 4, and so does a project record that is a symbolic link or any other
  non-file, and a file of the default set whose name holds a character that cannot be printed on
  a line (a line break, an escape, a direction override). A record written as
  `.github/actions-qa-ci.yaml` is refused with exit 2: the record has one name. Every diagnostic is escaped as it is written, and a project record's faults are
  listed one escaped line each, so a name or a line from the repository cannot forge a line or
  move the terminal; a diagnostic that cannot be written is exit 3.
- **`-t lane[,lane]`** runs only the named lanes (repeatable); an unknown name is refused with
  the lanes there are.
- **`--format text|agent|json|github`.** With none given, `ACTIONS_QA_FORMAT`, else `text` on a
  terminal, else `agent`. `github` is for the log of a workflow step: a workflow annotation
  (`::error file=…,line=…,col=…,title=<identifier>::message`) for each finding, at most ten of
  each level (error, warning, notice; GitHub does not document its limit, and a lower one than
  the real one loses nothing), then every finding as the `agent` format prints it, so the log
  keeps what the annotations leave out. A path that starts the way a workflow command does (`::`
  or `##[`) is printed quoted in every format, so a file name cannot become a command in the log.
- **`--sarif <file>`.** Also writes every finding as one SARIF 2.1.0 log with a single run, which
  GitHub code scanning accepts: the driver is `actions-qa-ci`, the bundled tools (actionlint,
  ShellCheck) are its extensions with their versions, each result carries our identifier as
  `ruleId` with the upstream identifier in its properties, and a finding covered by an exception
  stays in the log with an external suppression, so code scanning shows it as suppressed and does
  not drop it. The log is written whatever the findings are, so a clean run replaces an old log.
  The file is not written when nothing was analysed (a usage error, a refused record, no
  verdict); one that is asked for and cannot be written is exit 3. It is never written through a
  symbolic link or to a pipe, and a path inside the repository must not leave it through a
  link in one of its directories; a path outside the repository (absolute, or reaching out with
  `..`) is the operator's own and is only refused a symbolic link or a pipe at its end. A value
  that starts with `-` is refused, so a missing file name cannot swallow the next flag. The address
  of a rule's page in the log is on the default branch until the program has a release to name
  (`dbf.json`, `sarif-help-uri-not-at-release-tag`).
- **Exceptions.** An exception of the project record covers a finding with the same identifier
  and path, and the job and step if it names them (by id or by name, wherever in that job or step
  the finding's line falls within that job or step's own extent, so an `env:` after the steps is in
  the job and in no step); with no path it turns the defence off everywhere. A name that is the id
  of one job is that job, whatever else is named so; a name two jobs (or two steps of a job) share
  names none of them, so use the id. A covered finding stays in the output as excepted: counted,
  not listed, and in `--format json` with the exception that covers it. A job or step is found by
  reading the workflow. A workflow that cannot be read that way, or that uses a YAML alias, is not
  placed at all; jobs that share a line are not placed, and steps that share a line are placed in
  their job and in no step. An exception that names what is not placed matches nothing there. An
  id that two jobs (or two steps of a job) share names neither. In a full run (no `-t`, no file named) an exception that covers no finding is
  itself a finding, `actionsqaci.staleException`, at its line in the record.
- **`rules` shows the narrowing.** It reads the project record in the working directory, which is
  the repository root (run below it, with a record above, it says on standard error that it did
  not look), and refuses an invalid one with exit 2. Beside a defence the record turns off
  everywhere, the text format adds a `    narrowed: …` line and `rules --json` adds a `narrowedBy`
  array to that entry; an entry without one has no such key, so a consumer that reads entries as
  string maps breaks only when a record narrows something.
- **`rules --write-agent-summary <file>`** (or `=<file>`) puts the same list, narrowings
  included, in a region of the file between the lines `<!-- actions-qa-ci:begin -->` and
  `<!-- actions-qa-ci:end -->`, for the markdown file an agent reads (`CLAUDE.md`,
  `AGENTS.md`). The region is replaced where it is and appended where the file has none; the rest
  of the file is kept byte for byte, and a second run changes nothing. A file with a begin
  marker and no end marker (or more than one region) is refused with exit 2 and left alone, as
  is a link or anything but a regular file. A file whose lines end in CRLF gets the region with
  CRLF. A marker line is a marker wherever it is, a fenced code block included. The file is
  written beside itself and moved into place, with its permissions kept, so a write that fails
  leaves it as it was. The listing is still printed. `actionsqaci.agentSummaryCurrent` fails
  when a region in `CLAUDE.md` or `AGENTS.md` at the repository root is not the one the command
  would write now; a file with no region is not asked to have one.
- **Failure stops the run.** A lane that fails stops it with exit 3 and prints no findings: a
  partial list would read as a verdict.

### The project record

A project states what it has accepted in one file, `.github/actions-qa-ci.yml`: a `version` and a
list of `exceptions`, each with an `id`, optionally a `path` (with `job` and `step` to narrow it),
and a `hazard`, `scope` and `decision`. The loader refuses an unknown field, a duplicate key, a
second document, an alias, anchor, merge key or explicit tag (a hand-written record has no use for
them, and each is a way for a value to arrive from somewhere it is not written), a key that is
not plain text, an identifier no defence has and a path that is not a plain file path inside the
repository, and reports every fault at once. `actionsqaci.unjustifiedException` holds the three
justification fields to a standard (see its page); `rule actionsqaci.unjustifiedException
.github/actions-qa-ci.yml` runs it, and `check` runs it with the other lanes and applies the
exceptions (see above). The checks on the record itself, `actionsqaci.unjustifiedException` and
`actionsqaci.staleException`, cannot be excepted.

The record's `templates` section lists a `source` and its `copies`; `actionsqaci.templateDrift`
reports a copy that is not byte-identical to its source, and a source or copy that is missing or
is not a plain file, so a renamed copy cannot turn the check off. It compares whole files; a copy
that may vary in one key is not supported yet (`dbf.json`, `template-drift-whole-file-only`), and
a `mode` key is refused.

Its `requiredChecks` section lists the status checks a merge needs, each a `context` and
optionally a `branch`; `actionsqaci.requiredChecks` reports a context that no job of a workflow
triggered on `pull_request` produces, so a renamed job cannot leave branch protection waiting for
a check that never comes. With `check --online` it also compares the record with the branch
protection and rulesets the repository really has (the token in `GITHUB_TOKEN` or `GH_TOKEN`, the
repository in `GITHUB_REPOSITORY`, the API at `GITHUB_API_URL`, https only): a context the
record requires that the protection does not, and one the protection requires that the record
does not list, are findings. Nothing reaches the network without `--online`, and a protection
that cannot be read is exit 4 (`dbf.json`, `required-checks-online-comparison`). An exception
for `actionsqaci.requiredChecks`, at the record or everywhere, is called stale only by a run that
went online, as it may accept a finding only the comparison makes. A branch the record spells two ways (`default`
and its name) is one branch.

Its `analyse.unanalysed` section lists patterns of paths (`path`, with a `justification` holding a
`hazard` and a `scope`) of workflow-shaped files the project deliberately leaves out of the
analysis. `actionsqaci.lintCoverage` reports every tracked file that has a top-level `on` and
`jobs`, or is an `action.yml` with `runs`, that is neither analysed by default nor declared, and
a declaration that excuses nothing. It lists the tracked files by reading the repository's own
index (no `git` is run, and no other tool is needed), so a file not yet added is not seen; an
index it cannot read completely is exit 3. It is about the project, not one file, so `check` runs
it once whether or not the project has a record, and naming files on the command line runs only
the lanes that cover them. The patterns are the small glob of `internal/glob` (`*`, `?`, `**` as
a whole segment).

Its `analyse.include` section lists patterns of tracked `.yml` and `.yaml` files outside the
default set that the project wants analysed, such as a template it ships. Each file a pattern
names is linted by actionlint and guarded against inline suppression as a workflow, or only
guarded as an action when it is named `action.yml` or `action.yaml`. A pattern that names no
such file is an `actionsqaci.lintCoverage` finding. The other sections the design gives the
record arrive with the checks that use them.

### Exit codes

| Code | Meaning                                                              |
| ---- | -------------------------------------------------------------------- |
| 0    | analysed, nothing to report                                          |
| 1    | analysed, findings reported                                          |
| 2    | the command line or a project record was wrong; nothing was analysed |
| 3    | the toolchain itself failed: a rule, a bundled tool or its pinned binary, a page the build should hold, or output that could not be written |
| 4    | the analysis could not reach a verdict: a file it should cover could not be read as a plain file, the branch protection `--online` asked for could not be read, or the process a lane ran in crashed, was killed, wrote too much or ran past its time limit |

A usage error writes nothing to standard output: it is not a verdict.

`check` runs the actionlint lane on each file in a process of its own (this program started with
an internal `lane` command), with a time limit of two minutes and a cap on what it may write. A
workflow made to send actionlint's expression parser into unbounded recursion, or to make it
slow, therefore ends that process and not the run: the run has no verdict, names the lane and
the file, and exits 4. A lane process whose lane fails on its own account is exit 3. What a
lane process started, if it stays in the lane process's process group (ShellCheck does), ends
with it, and a lane process ends if `actions-qa` itself is killed.

One declared gap (`dbf.json`, `actionlint-runs-in-process`): the `rule` harness still runs
actionlint inside its own process, so there a workflow built to recurse without bound ends the
program with the Go runtime's own exit code 2, and one built to be slow runs without a time
limit.

## Conformance

`dbf.json` declares the method and toolchain specification versions this repository is audited
against, and the known gaps it states rather than hides. A declaration with an unknown field, a
missing field or data after the object is refused. `actions-qa version --json` prints the
embedded copy, and a test pins the versions so a change is deliberate.

## Contributing

Read [CLAUDE.md](CLAUDE.md) first. Every change that a consumer could notice carries an
`## Unreleased` entry in [CHANGELOG.md](CHANGELOG.md).

```bash
gofmt -l .
go vet ./...
go test ./...
```

## Licence

See [LICENSE](LICENSE).
