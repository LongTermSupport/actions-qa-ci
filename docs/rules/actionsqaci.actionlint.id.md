# actionsqaci.actionlint.id

## What it forbids or requires

Job IDs and step IDs are unique within their scope, start with a letter or `_`, and contain only
letters, digits, `-` and `_`.

## Hazard

A duplicate or malformed ID makes `needs:` and `steps.<id>.outputs` references invalid or
ambiguous.

## Correct construction

```yaml
jobs:
  build-1:
    runs-on: ubuntu-latest
    steps:
      - id: version
        run: echo "value=1" >> "$GITHUB_OUTPUT"
```

## Exceptions

An exception is recorded in the project record with the file, this identifier and a justification.
The project record is not part of this build yet, so no exception can be recorded today: rename the
ID.

## Upstream

actionlint v1.7.12, kind `id`:
<https://github.com/rhysd/actionlint/blob/v1.7.12/docs/checks.md#job-id-and-step-id-uniqueness>
