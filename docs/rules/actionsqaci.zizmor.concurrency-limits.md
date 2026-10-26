# actionsqaci.zizmor.concurrency-limits

## What it forbids or requires

A workflow states a `concurrency` group with `cancel-in-progress: true`, or with an expression
that is true in most cases. A workflow with no suitable `concurrency` setting is reported.

zizmor reports this at the pedantic persona. The lane runs the regular persona, so `actions-qa
check` reports it only where the project record's `calibrations.enable` names this identifier; this
rule's own harness always shows it. The audit works offline.

## Hazard

A run that a newer one has superseded goes on consuming billed runner time. Two runs of the same
workflow can also race, for example when each looks up an artifact by workflow and job
identifiers and finds the other's.

## Correct construction

```yaml
name: x
on: push
permissions: {}
concurrency:
  group: x-${{ github.ref }}
  cancel-in-progress: true
jobs:
  a:
    name: a
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      - name: hi
        run: echo hi
```

## Exceptions

A workflow whose runs must all finish, such as a deployment that must not be cut off half way, sets
`cancel-in-progress: false` and keeps the group so that runs queue; that is a decision to record
in the project record with the file, this identifier, the hazard, the scope and the decision,
held to the standard of `actionsqaci.unjustifiedException`.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#concurrency-limits>
