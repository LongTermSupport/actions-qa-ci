# actionsqaci.zizmor.anonymous-definition

## What it forbids or requires

Every workflow and every composite action definition has a top-level `name:`. The audit is of
zizmor's pedantic persona, because a missing name has no security impact of its own. The lane runs
the regular persona, so `actions-qa check` reports it only where the project record's
`calibrations.enable` names this identifier; this rule's own harness always shows it. It works
offline.

## Hazard

A workflow with no name is shown in the Actions interface and in status checks under its file
path, which a reader cannot tell apart from a similar file. The audit is pedantic because the
omission has no security impact of its own: it costs a reader clarity.

## Correct construction

```yaml
name: Echo Test
on: push
permissions: {}

jobs:
  build:
    name: build
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      - run: echo "Hello!"
```

## Exceptions

A generated workflow whose file name is its name is recorded in the project record,
`.github/actions-qa-ci.yml`, with this identifier, the file, a `hazard`, a `scope` and a `decision`
(see `actionsqaci.unjustifiedException`). A `# zizmor: ignore` comment is refused by
`actionsqaci.inlineSuppression`.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#anonymous-definition>
