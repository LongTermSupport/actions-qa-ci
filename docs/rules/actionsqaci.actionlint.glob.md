# actionsqaci.actionlint.glob

## What it forbids or requires

The branch, tag and path filters under `on:` are valid GitHub filter patterns.

## Hazard

An invalid pattern does not select what the author meant, so the filter includes or excludes the
wrong refs and paths.

## Correct construction

```yaml
on:
  push:
    branches: [main, "release/**"]
```

## Exceptions

An exception is recorded in the project record with the file, this identifier and a justification.
The project record is not part of this build yet, so no exception can be recorded today: correct
the pattern.

## Upstream

actionlint v1.7.12, kind `glob`:
<https://github.com/rhysd/actionlint/blob/v1.7.12/docs/checks.md#glob-filter-pattern-syntax-validation>
