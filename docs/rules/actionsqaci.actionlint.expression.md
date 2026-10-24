# actionsqaci.actionlint.expression

## What it forbids or requires

Every `${{ }}` expression parses, refers only to contexts, properties and functions that exist where
it is written, and uses operands of types the operator accepts.

actionlint also warns that an expression in a script interpolates a context an outside contributor
controls. That is a different hazard with its own owner,
`actionsqaci.zizmor.template-injection`, and the lane drops actionlint's "potentially untrusted"
messages so that the hazard has one identifier.

## Hazard

An expression that does not type-check is wrong at run time, so a condition or a value is not what
the author wrote.

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
expression.

## Upstream

actionlint v1.7.12, kind `expression`:
<https://github.com/rhysd/actionlint/blob/v1.7.12/docs/checks.md>
