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
go run ./cmd/actions-qa rule <id> <path> # one rule on one file, only its findings (--format text|agent|json)
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

The lane also runs ShellCheck (v0.11.0) over the `bash` and `sh` scripts in `run:` steps, through
actionlint's integration, and reports each finding as `actionsqaci.shellcheck.SC<nnnn>`; all
codes share the page `docs/rules/actionsqaci.shellcheck.md`. Only `error` and `warning` findings
are reported by default. The ShellCheck binary is the one pinned in `internal/toolchain/manifest.json`
by sha256, looked for beside the program (or in the directory named by `ACTIONS_QA_CI_TOOLS_DIR`)
and verified before every run; one that is missing or differs fails the lane with exit 3, and a
ShellCheck on `PATH` is never used. In a development checkout, `go run ./cmd/actions-qa-dev fetch
bin` downloads it, and `ACTIONS_QA_CI_TOOLS_DIR=$PWD/bin` points the program at it. pyflakes
integration is not enabled.

### The check command

`actions-qa check` validates the project record, then runs the lanes (`actionlint`, which carries
ShellCheck's findings, and `firstParty`, which checks the record) over the files the project has
and prints every finding. It must be run from the repository root, which holds `.git`: from
anywhere else it would find nothing and read as clean, so it refuses.

- **Files.** By default: the workflows directly in `.github/workflows`, every `action.yml` or
  `action.yaml` under `.github/actions` and at the root, `.github/dependabot.yml` or
  `.github/dependabot.yaml`, and the project record. A lane analyses the files it covers; today
  actionlint covers the workflows and the first-party lane covers the record. A file of the
  default set that no lane of the build covers is named on standard error, so a clean result is
  not read as covering it. Naming files on the command line analyses only those, and
  a file no chosen lane covers is refused rather than passed over. A symbolic link or other
  non-file where the default set looks (a directory named like a workflow or an action, at any depth) stops the run with exit 4, and so does a project record that is a symbolic link or any other
  non-file, and a file of the default set whose name holds a character that cannot be printed on
  a line (a line break, an escape, a direction override). A record written as
  `.github/actions-qa-ci.yaml` is refused with exit 2: the record has one name. Every diagnostic is escaped as it is written, and a project record's faults are
  listed one escaped line each, so a name or a line from the repository cannot forge a line or
  move the terminal; a diagnostic that cannot be written is exit 3.
- **`-t lane[,lane]`** runs only the named lanes (repeatable); an unknown name is refused with
  the lanes there are.
- **`--format text|agent|json`.** With none given, `ACTIONS_QA_FORMAT`, else `text` on a
  terminal, else `agent`.
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
.github/actions-qa-ci.yml` runs it, and `check` runs it with the other lanes. This build reads
and checks the record; it does not yet apply an exception to a finding, and the other sections the design gives the record arrive with the
checks that use them.

### Exit codes

| Code | Meaning                                                              |
| ---- | -------------------------------------------------------------------- |
| 0    | analysed, nothing to report                                          |
| 1    | analysed, findings reported                                          |
| 2    | the command line or a project record was wrong; nothing was analysed |
| 3    | the toolchain itself failed: a rule, a bundled tool or its pinned binary, a page the build should hold, or output that could not be written |
| 4    | the analysis could not reach a verdict: a file it should cover could not be read as a plain file |

A usage error writes nothing to standard output: it is not a verdict.

One declared gap (`dbf.json`, `actionlint-runs-in-process`): actionlint runs inside the program,
so a workflow that sends its expression parser into unbounded recursion ends the program with
the Go runtime's own exit code 2, which is not a usage error, and an expression made to be slow
runs without a time limit. Neither is reported as exit 4 yet.

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
