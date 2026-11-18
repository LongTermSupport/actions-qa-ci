# actionsqaci.actionlint.runner-label

## What it forbids or requires

`runs-on` names a runner label that exists: a GitHub-hosted label, one of the preset labels of a
self-hosted runner, or a custom label the project declares under `self-hosted-runner: labels:` in
its actionlint configuration, `.github/actionlint.yaml` (or `.github/actionlint.yml` when there is
no `.yaml`). Those labels are the only part of that file the lane applies; its ignores and
configuration variables are not.

## Hazard

A misspelled label waits forever for a runner that does not exist, so the job queues until it times
out.

## Correct construction

```yaml
jobs:
  build:
    runs-on: ubuntu-latest
```

A job on a self-hosted pool names labels the project declares once:

```yaml
# .github/actionlint.yaml
self-hosted-runner:
  labels:
    - linux-pool
```

```yaml
jobs:
  build:
    runs-on: [self-hosted, linux-pool]
```

## Exceptions

None needed for a custom label: declare it in the actionlint configuration, which names the
project's runner pool once rather than once per workflow file. A label the configuration does not
declare is reported.

## Upstream

actionlint v1.7.12, kind `runner-label`:
<https://github.com/rhysd/actionlint/blob/v1.7.12/docs/checks.md#runner-labels>
