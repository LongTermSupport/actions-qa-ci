# actions-qa-ci

actions-qa-ci is the [Defence Before Fix](https://defence-before-fix.github.io/) toolchain for
GitHub Actions code. It is a Go program, `actions-qa`, built as one static binary that bundles
its rule backends. This file holds the binding floor; the depth is one link away.

## Binding floor

- **Defence Before Fix**: found a defect? Do not fix it yet; build the detector for its class
  first. A new check is a defence: a stable identifier, a page stating the correct construction,
  an entry in the identifier index (`docs/rules/README.md`, generated) and a place in
  `actions-qa rules` (the registry in `internal/catalogue`). A suppression, a baseline or a
  narrowing is an Owner decision, never a silent edit; a gap found is closed, not accepted.
- **Test first**: a failing test, or a committed detector, comes before the code that satisfies
  it. A test never discards an error with the blank identifier; it reads or asserts every value
  it receives.
- **No hidden failures**: an error is returned or reported by name, never swallowed. A tool
  that fails is exit 3, an analysis with no verdict is exit 4, and neither is ever reported as
  clean. (Exit 3 is used today for a failing rule or lane, a missing page or unwritable output;
  exit 4 for a file in the default set that cannot be read as a plain file, and for a lane
  process that crashed, was killed, wrote too much or ran past its time limit.)
- **One owner per hazard class**: a hazard is reported by exactly one rule. A bundled backend's
  finding is renamed to this project's stable identifier, and a duplicate is dropped.
- **Conformance is declared, not claimed**: `dbf.json` states the method and toolchain versions
  this repository is audited against and every known gap. The version test pins them; moving
  either is a deliberate change made after the specification itself moves.
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

The check that gates a merge is the `CI` job of `.github/workflows/ci.yml`; making it a required
status check through branch protection is the Owner's setting, and is not yet in place. Open a pull request from a
branch named `feature/…`, `bugfix/…` or `chore/…`; the default branch is `main`.

## Where to look

- Layout: `cmd/actions-qa` is the entry point and holds no logic; `internal/cli` is the command
  line; the root package holds what the module declares about itself.
- Pinned third-party binaries: `internal/toolchain/manifest.json` pins each release by archive and
  binary sha256, the runtime refuses a binary that differs, and `go run ./cmd/actions-qa-dev
  fetch bin` downloads the pinned ones for development.
- The `check` run: `internal/check` discovers the default file set and runs the lanes, which
  `internal/catalogue` lists; `internal/cli/check.go` is the command. A lane marked `Isolated`
  runs in a child process (`internal/check/isolate.go`, started through the hidden `lane`
  command in `internal/cli/lane.go`), so a backend that crashes or hangs is exit 4, not a dead
  program.
- Exceptions: `internal/locate` finds the job and step of a line, `internal/exceptions` applies
  the record's exceptions to findings and reports the stale ones.
- The project record: `internal/record` loads `.github/actions-qa-ci.yml` and judges
  justifications; `internal/firstparty` holds the checks this toolchain makes itself.
- Tracked files and patterns: `internal/gitindex` lists the files the repository tracks by reading
  its index (no `git` is run), and `internal/glob` is the small pattern language the record uses.
  A lane marked `Subject` is about the project, not one file, and runs once per whole-project run.
- Exit codes: [README.md](README.md#exit-codes).
- Conformance: `dbf.json`, printed by `actions-qa version --json`.
