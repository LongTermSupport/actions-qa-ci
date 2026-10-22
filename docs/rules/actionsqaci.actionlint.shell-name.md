# actionsqaci.actionlint.shell-name

## What it forbids or requires

A step's `shell:` names a shell GitHub provides, or is a custom command line that carries the `{0}`
placeholder for the script file.

## Hazard

An unknown shell name makes the step fail on the runner, or run under a different interpreter from
the one the script was written for.

## Correct construction

```yaml
steps:
  - run: echo hi
    shell: bash
```

## Exceptions

An exception is recorded in the project record with the file, this identifier and a justification
that names the shell and the runner that provides it. The project record is not part of this build
yet, so no exception can be recorded today: use a shell GitHub provides.

## Upstream

actionlint v1.7.12, kind `shell-name`:
<https://github.com/rhysd/actionlint/blob/v1.7.12/docs/checks.md#shell-name-validation-at-shell>
