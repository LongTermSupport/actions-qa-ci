# actionsqaci.actionlint.deprecated-commands

## What it forbids or requires

A `run:` script does not use the `set-output`, `save-state`, `set-env` or `add-path` workflow
commands. Outputs and state are appended to the files `GITHUB_OUTPUT` and `GITHUB_STATE`, and
environment and path changes to `GITHUB_ENV` and `GITHUB_PATH`.

## Hazard

GitHub has disabled these commands, so the value is silently never set.

## Correct construction

```yaml
steps:
  - id: version
    run: echo "value=1" >> "$GITHUB_OUTPUT"
```

## Exceptions

An exception is recorded in the project record with the file, this identifier and a justification.
The project record is not part of this build yet, so no exception can be recorded today: use the
environment files.

## Upstream

actionlint v1.7.12, kind `deprecated-commands`:
<https://github.com/rhysd/actionlint/blob/v1.7.12/docs/checks.md#check-deprecated-workflow-commands>
