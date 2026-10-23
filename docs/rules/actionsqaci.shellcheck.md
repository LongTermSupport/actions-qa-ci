# actionsqaci.shellcheck.SC<nnnn>

## What it forbids or requires

The shell in a `run:` step has no ShellCheck error or warning. Each finding is reported under its
own ShellCheck code, `actionsqaci.shellcheck.SC` and the four digits (for example
`actionsqaci.shellcheck.SC2115`), and every code shares this page.

What is checked, and what is not:

- Only steps that run under `bash` or `sh` are checked. A step whose shell is `pwsh`, `python`,
  `cmd` or anything else is not, and a job whose `runs-on:` names a Windows runner by a literal
  label defaults to `pwsh`. A `runs-on:` that is an expression (`${{ matrix.os }}`) is not read,
  so a PowerShell step in such a job is checked as bash and can draw false findings: give it an
  explicit `shell: pwsh`.
- The script is checked the way GitHub runs it: with `set -eo pipefail` first for bash and
  `set -e` first for sh, and with each `${{ }}` expression replaced by underscores.
- Findings below `warning` (ShellCheck's `info` and `style` levels) are not reported. Lowering
  that threshold is a decision for the Owner, not a default.
- actionlint, which runs ShellCheck here, always excludes SC1091, SC2043, SC2050, SC2153,
  SC2154, SC2157 and SC2194, because they misfire on workflow scripts. They are never reported.
- A finding is positioned at the `run:` key. The line and column inside the script are in the
  message, counted from the first line of the script, except that a `${{ }}` expression spanning
  several lines counts as one, so a later line is reported earlier than it is.
- ShellCheck's own `# shellcheck disable=` directive inside a script is honoured and suppresses
  the finding without a record. This build does not report such a directive yet (a known gap in
  `dbf.json`); the environment variable `SHELLCHECK_OPTS` and a `.shellcheckrc` are ignored.
- The family accepts any code from SC1000 to SC3999, including one ShellCheck never emits, which
  simply never fires: a mistyped code reads as clean (also a known gap).

## Hazard

A shell defect changes what a CI step does when a value is empty, contains a space or a glob
character, or is attacker-influenced: an unquoted expansion splits and globs, and
`rm -rf $DIR/*` becomes `rm -rf /*` when `DIR` is empty.

## Correct construction

The code decides the fix, and the ShellCheck page for the code (linked below) states it. The
shape is the same for most of them: put the value in `env:`, quote every expansion, and guard
the ones that must not be empty.

```yaml
steps:
  - run: rm -rf "${BUILD_DIR:?}"/*
    env:
      BUILD_DIR: ${{ runner.temp }}/build
```

## Exceptions

An exception is recorded in the project record with the file, the exact code identifier and a
justification that names why the construction cannot be changed. The project record is not part
of this build yet, so no exception can be recorded today: change the script.

## Upstream

ShellCheck at the release `internal/toolchain/manifest.json` pins, run by actionlint v1.7.12 through its ShellCheck integration:
<https://github.com/rhysd/actionlint/blob/v1.7.12/docs/checks.md#check-shellcheck-integration>.

The page for a code is at `https://www.shellcheck.net/wiki/SC` followed by the four digits, for
example <https://www.shellcheck.net/wiki/SC2115>.
