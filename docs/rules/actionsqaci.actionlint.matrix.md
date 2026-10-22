# actionsqaci.actionlint.matrix

## What it forbids or requires

A job matrix lists each value once, and every `include` or `exclude` entry refers to a key and a
value the matrix defines.

## Hazard

A repeated value runs the same job twice, and an `exclude` that matches nothing hides a combination
the author believed was removed.

## Correct construction

```yaml
strategy:
  matrix:
    os: [ubuntu-latest, macos-latest]
```

## Exceptions

An exception is recorded in the project record with the file, this identifier and a justification
that names why the repetition is intended. The project record is not part of this build yet, so no
exception can be recorded today: fix the matrix.

## Upstream

actionlint v1.7.12, kind `matrix`:
<https://github.com/rhysd/actionlint/blob/v1.7.12/docs/checks.md#matrix-values>
