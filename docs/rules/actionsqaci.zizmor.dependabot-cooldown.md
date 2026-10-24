# actionsqaci.zizmor.dependabot-cooldown

## What it forbids or requires

Every update in `.github/dependabot.yml` has a `cooldown` whose `default-days` is at least the
threshold, which is 7 days unless configured otherwise. The audit reads the Dependabot
configuration, not a workflow. It runs at zizmor's regular persona, which is what the lane runs
by default, and works offline.

## Hazard

With no cooldown Dependabot proposes a release the day it is published. A release that was
compromised is then proposed, and merged, before it has been noticed and withdrawn.

## Correct construction

```yaml
version: 2
updates:
  - package-ecosystem: pip
    directory: /
    schedule:
      interval: daily
    cooldown:
      default-days: 7
```

## Exceptions

An ecosystem that must follow a vendor's release the day it is published, for instance a security
feed, is recorded in the project record, `.github/actions-qa-ci.yml`, with this identifier, the
file, a `hazard`, a `scope` and a `decision` (see `actionsqaci.unjustifiedException`). A
`# zizmor: ignore` comment is refused by `actionsqaci.inlineSuppression`.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#dependabot-cooldown>
