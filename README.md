# actions-qa-ci

actions-qa-ci is the [Defence Before Fix](https://defence-before-fix.github.io/) toolchain for
GitHub Actions code: workflows, composite actions and the shell they run. It is one static
program, `actions-qa`, that bundles its rule backends and gives every finding a stable
identifier and a page stating the correct construction.

This repository is being built in phases. What exists today is the repository floor: the Go
module, the conformance declaration, the command line skeleton and the repository's own CI.
The rules arrive in later phases.

## Requirements

- Go, to build from source. A released bundle needs nothing on the host.

## Use

```bash
go run ./cmd/actions-qa version          # the specification versions this build is audited against
go run ./cmd/actions-qa version --json   # the same, as the conformance declaration
go run ./cmd/actions-qa rules            # every active rule, without running it (--json for a list)
go run ./cmd/actions-qa rule-doc <id>    # the page for a rule identifier, offline
go run ./cmd/actions-qa help
```

Every rule has an identifier, and every identifier has a page under `docs/rules/` that states
the correct construction. `docs/rules/README.md` is the identifier index; it is generated from
the registry and a test fails when it is stale (`go test . -run TestTheIdentifierIndexIsCurrent
-update` regenerates it). A test also fails when a rule has no complete page, or a page has no
rule. No rule ships yet: the registry is empty until the bundled backends arrive.

### Exit codes

| Code | Meaning                                                              |
| ---- | -------------------------------------------------------------------- |
| 0    | analysed, nothing to report                                          |
| 1    | analysed, findings reported                                          |
| 2    | the command line or a project record was wrong; nothing was analysed |
| 3    | a bundled tool failed or its pinned binary did not verify            |
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
