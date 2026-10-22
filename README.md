# actions-qa-ci

actions-qa-ci is the [Defence Before Fix](https://defence-before-fix.github.io/) toolchain for
GitHub Actions code: workflows, composite actions and the shell they run. It is one static
program, `actions-qa`, that bundles its rule backends and gives every finding a stable
identifier and a page stating the correct construction.

This repository is being built in phases. What exists today is the core: the Go module, the
conformance declaration, the finding model and identifier scheme, the rule registry, the output
formats, the `version`, `rules`, `rule-doc` and `rule` commands, and the repository's own CI.
The first backend is compiled in: actionlint, as sixteen rules, with ShellCheck running on the
shell in `run:` steps. The remaining backends and the first-party checks arrive in later phases,
and there is no `check` command yet, so the rules are reachable through `rule <id> <path>`.

## Requirements

- Go, to build from source. A released bundle needs nothing on the host.

## Use

```bash
go run ./cmd/actions-qa version          # the specification versions this build is audited against
go run ./cmd/actions-qa version --json   # the same, as the conformance declaration
go run ./cmd/actions-qa rules            # every active rule, without running it (--json for a list)
go run ./cmd/actions-qa rule-doc <id>    # the page for a rule identifier, offline
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

### The project record

A project states what it has accepted in one file, `.github/actions-qa-ci.yml`: a `version` and a
list of `exceptions`, each with an `id`, optionally a `path` (with `job` and `step` to narrow it),
and a `hazard`, `scope` and `decision`. The loader refuses an unknown field, a duplicate key, a
second document, an identifier no defence has and a path that is not a plain file path inside the
repository, and reports every fault at once. `actionsqaci.unjustifiedException` holds the three
justification fields to a standard (see its page); `rule actionsqaci.unjustifiedException
.github/actions-qa-ci.yml` runs it. This build reads and checks the record; it does not yet apply
an exception to a finding, and the other sections the design gives the record arrive with the
checks that use them.

### Exit codes

| Code | Meaning                                                              |
| ---- | -------------------------------------------------------------------- |
| 0    | analysed, nothing to report                                          |
| 1    | analysed, findings reported                                          |
| 2    | the command line or a project record was wrong; nothing was analysed |
| 3    | the toolchain itself failed: a rule, a bundled tool or its pinned binary, a page the build should hold, or output that could not be written |
| 4    | the analysis could not reach a verdict                               |

A usage error writes nothing to standard output: it is not a verdict.

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
