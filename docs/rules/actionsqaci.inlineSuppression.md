# actionsqaci.inlineSuppression

## What it forbids or requires

A workflow or composite action holds no comment that tells a tool to leave a finding out. Three
forms are refused, wherever in the file they appear, in any letter case and with or without
spaces around the colon:

- `# zizmor: ignore` and `# zizmor: ignore[audit-name]`, with or without a reason after it;
- `# ratchet:exclude`;
- `# shellcheck disable=SC…`, including one that carries another option first, such as
  `# shellcheck source=/dev/null disable=SC1090`, in a `run:` script or in the YAML around it.

One finding is reported for each such comment, at the comment. The check reads the text of the
file, not its meaning, so the same words inside a string or an `echo` are refused too: write
them some other way.

## Hazard

A comment at the call site suppresses with nothing behind it: no hazard written down, no scope,
no decision, and no way for a reviewer or `actionsqaci.staleException` to find it again when the
reason goes away. The three differ in what they do today. A `# shellcheck disable=` is honoured
by the bundled ShellCheck, so it hides a finding in a script the rest of the toolchain never
sees. This toolchain runs zizmor with its ignores switched off, so a `# zizmor: ignore` is
inert here, which leaves a comment that reads as if it worked and is honoured by anyone who runs
zizmor bare. ratchet is not bundled yet.

## Correct construction

Fix the finding. If the finding is accepted, put the acceptance in the project record, where it
has a hazard, a scope and a decision, and is reported when it stops matching anything:

```yaml
version: 1
exceptions:
  - id: actionsqaci.shellcheck.SC2086
    path: .github/workflows/release.yml
    job: release
    step: tag
    hazard: "An unquoted variable that is split on purpose to pass several flags to one command"
    scope: "Only the tag step, whose variable is built from a fixed list in the same step"
    decision: https://github.com/LongTermSupport/php-qa-ci/issues/123
```

## Exceptions

An exception for this check is an exception to the rule that suppressions are made in one place,
so it is held to the standard of the record like any other. Prefer removing the comment.

## Upstream

First-party, from the Defence Before Fix toolchain specification's requirement that a defence
forbid every suppression route that bypasses the project record. The comment forms are those
documented by zizmor (<https://docs.zizmor.sh/usage/#ignoring-results>), ratchet
(<https://github.com/sethvargo/ratchet>) and ShellCheck
(<https://www.shellcheck.net/wiki/Directive>).
