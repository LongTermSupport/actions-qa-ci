# actionsqaci.actionlint.expression

## What it forbids or requires

Every `${{ }}` expression parses, refers only to contexts, properties and functions that exist where
it is written, and uses operands of types the operator accepts.

## Hazard

An expression that does not type-check evaluates to an empty string or fails at run time, so a
condition or a value is silently wrong.

## Correct construction

```yaml
steps:
  - run: echo "$EVENT"
    env:
      EVENT: ${{ github.event_name }}
```

## Exceptions

An exception is recorded in the project record with the file, this identifier and a justification.
The project record is not part of this build yet, so no exception can be recorded today: correct the
expression. Untrusted input inside an expression is a different hazard and has its own rule.

## Upstream

actionlint v1.7.12, kind `expression`:
<https://github.com/rhysd/actionlint/blob/v1.7.12/docs/checks.md>
