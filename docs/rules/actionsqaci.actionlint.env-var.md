# actionsqaci.actionlint.env-var

## What it forbids or requires

Environment variable names under `env:` contain no space, `=` or `&`.

## Hazard

A name with those characters cannot be exported by a shell, so the variable is missing or the step
fails.

## Correct construction

```yaml
env:
  BUILD_MODE: release
```

## Exceptions

An exception is recorded in the project record with the file, this identifier and a justification.
The project record is not part of this build yet, so no exception can be recorded today: rename the
variable.

## Upstream

actionlint v1.7.12, kind `env-var`:
<https://github.com/rhysd/actionlint/blob/v1.7.12/docs/checks.md#environment-variable-names>
