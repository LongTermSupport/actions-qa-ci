# actionsqaci.actionlint.runner-label

## What it forbids or requires

`runs-on` names a runner label that exists: a GitHub-hosted label or one of the preset labels of a
self-hosted runner. The project's own actionlint list of custom runner labels is not applied, so
the message's suggestion to add a label to `actionlint.yaml` does not apply here.

## Hazard

A misspelled label waits forever for a runner that does not exist, so the job queues until it times
out.

## Correct construction

```yaml
jobs:
  build:
    runs-on: ubuntu-latest
```

## Exceptions

A custom self-hosted label is recorded in the project record with the file, this identifier and a
justification that names the runner pool that carries the label. The project record is not part of
this build yet, so no exception can be recorded today and a custom label is reported.

## Upstream

actionlint v1.7.12, kind `runner-label`:
<https://github.com/rhysd/actionlint/blob/v1.7.12/docs/checks.md#runner-labels>
