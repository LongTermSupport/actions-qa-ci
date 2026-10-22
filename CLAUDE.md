# actions-qa-ci

actions-qa-ci is the [Defence Before Fix](https://defence-before-fix.github.io/) toolchain for
GitHub Actions code. It is a Go program, `actions-qa`, built as one static binary that bundles
its rule backends. This file holds the binding floor; the depth is one link away.

## Binding floor

- **Defence Before Fix**: found a defect? Do not fix it yet; build the detector for its class
  first. A new check is a defence: a stable identifier, a page stating the correct construction,
  an entry in the identifier index and a place in `actions-qa rules`. A suppression, a baseline
  or a narrowing is an Owner decision, never a silent edit; a gap found is closed, not accepted.
- **Test first**: a failing test, or a committed detector, comes before the code that satisfies
  it. A test never discards an error with the blank identifier; it reads or asserts every value
  it receives.
- **No hidden failures**: an error is returned or reported by name, never swallowed. A tool
  that fails is exit 3, an analysis with no verdict is exit 4, and neither is ever reported as
  clean.
- **One owner per hazard class**: a hazard is reported by exactly one rule. A bundled backend's
  finding is renamed to this project's stable identifier, and a duplicate is dropped.
- **Conformance is declared, not claimed**: `dbf.json` states the method and toolchain versions
  this repository is audited against and every known gap. The version test pins them; moving
  either is a deliberate change made after the vendored specification moves.
- **Releases come from the changelog**: never choose a version or cut a tag by hand. A change a
  consumer can notice carries an `## Unreleased` entry in `CHANGELOG.md`.
- **The repository passes its own checks**: every workflow has `permissions: {}` at the top
  with per-job grants, `concurrency` with cancel-in-progress, `uses:` pinned to a full commit
  SHA, and `persist-credentials: false` on checkout.
- **Public by default**: this repository is public. No secret, credential, internal host or
  private path belongs in a file, a commit message or an issue.

## Working

```bash
gofmt -l .        # prints nothing when formatted
go vet ./...
go test ./...
```

The required check is the `CI` job of `.github/workflows/ci.yml`. Open a pull request from a
branch named `feature/…`, `bugfix/…` or `chore/…`; the default branch is `main`.

## Where to look

- Layout: `cmd/actions-qa` is the entry point and holds no logic; `internal/cli` is the command
  line; the root package holds what the module declares about itself.
- Exit codes: [README.md](README.md#exit-codes).
- Conformance: `dbf.json`, printed by `actions-qa version --json`.
