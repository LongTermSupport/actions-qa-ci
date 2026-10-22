# actionsqaci.actionlint.if-cond

## What it forbids or requires

An `if:` condition is either a plain expression or one `${{ }}` that holds the whole condition. Text
around `${{ }}` and conditions that are always true or always false are refused.

## Hazard

A condition with characters around `${{ }}` is a non-empty string, which is always true, so the
guard never guards.

## Correct construction

```yaml
steps:
  - if: ${{ github.event_name == 'push' }}
    run: echo hi
```

## Exceptions

An exception is recorded in the project record with the file, this identifier and a justification.
The project record is not part of this build yet, so no exception can be recorded today: write the
whole condition inside one expression.

## Upstream

actionlint v1.7.12, kind `if-cond`:
<https://github.com/rhysd/actionlint/blob/v1.7.12/docs/checks.md#constant-conditions-at-if>
