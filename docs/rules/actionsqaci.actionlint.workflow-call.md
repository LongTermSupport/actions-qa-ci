# actionsqaci.actionlint.workflow-call

## What it forbids or requires

A job that calls a reusable workflow passes every input and secret the callee requires, passes no
input or secret it does not declare, and reads only outputs it declares.

## Hazard

A missing required input fails the called workflow at run time, and an undeclared one is rejected or
ignored.

## Correct construction

```yaml
jobs:
  call:
    uses: ./.github/workflows/callee.yml
    with:
      x: hello
```

## Exceptions

An exception is recorded in the project record with the file, this identifier and a justification.
The project record is not part of this build yet, so no exception can be recorded today: pass what
the callee declares.

## Upstream

actionlint v1.7.12, kind `workflow-call`:
<https://github.com/rhysd/actionlint/blob/v1.7.12/docs/checks.md#reusable-workflows>
