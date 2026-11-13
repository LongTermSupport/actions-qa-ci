# actionsqaci.zizmor.unsound-contains

## What it forbids or requires

A workflow does not call `contains()` with a string as the thing searched. Given a string,
`contains()` is a substring test, so `contains('refs/heads/main refs/heads/develop', github.ref)`
is also true for a ref such as `refs/heads/mai`. Search a list instead, by
`contains(fromJSON('["refs/heads/main", "refs/heads/develop"]'), github.ref)`, or compare for
equality with each value and join the comparisons with `||`. This audit applies to workflows, not
to composite actions. It runs at the regular persona, so the lane reports it by default, and
needs no network.

## Hazard

An allow-list written as a string admits every substring of it. A branch, tag or name an attacker
can choose may satisfy a guard that was meant to accept only the listed values.

## Correct construction

```yaml
steps:
  - name: maybe
    if: contains(fromJSON('["refs/heads/main", "refs/heads/develop"]'), github.ref)
    run: echo hi
```

## Exceptions

None for a list of fixed values. If a substring test is the intent, record it in the project
record, `.github/actions-qa-ci.yml`, with the file, this identifier, the job and step, and a
hazard, scope and decision that say what the string and the searched value are.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#unsound-contains>
