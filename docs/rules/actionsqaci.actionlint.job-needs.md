# actionsqaci.actionlint.job-needs

## What it forbids or requires

Every job named in `needs:` exists in the same workflow, and the dependencies form no cycle.

## Hazard

A dependency on a missing job makes the workflow invalid, and a cycle means no job in it can ever
start.

## Correct construction

```yaml
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo build
  test:
    runs-on: ubuntu-latest
    needs: [build]
    steps:
      - run: echo test
```

## Exceptions

An exception is recorded in the project record with the file, this identifier and a justification.
The project record is not part of this build yet, so no exception can be recorded today: name a job
that exists.

## Upstream

actionlint v1.7.12, kind `job-needs`:
<https://github.com/rhysd/actionlint/blob/v1.7.12/docs/checks.md#job-dependencies-validation>
