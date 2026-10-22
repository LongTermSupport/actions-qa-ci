# actionsqaci.actionlint.syntax-check

## What it forbids or requires

A workflow file is valid GitHub Actions workflow syntax: every key is one the section allows, every
required key is present, and no key or mapping is empty, duplicated or of the wrong type.

## Hazard

GitHub rejects the workflow, or silently ignores the misplaced key, so the job does not behave as
written.

## Correct construction

```yaml
name: ci
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo hi
```

## Exceptions

An exception is recorded in the project record with the file, this identifier and a justification
that names why the file is meant to be invalid. The project record is not part of this build yet, so
no exception can be recorded today: fix the file.

## Upstream

actionlint v1.7.12, kind `syntax-check`:
<https://github.com/rhysd/actionlint/blob/v1.7.12/docs/checks.md#unexpected-keys>
