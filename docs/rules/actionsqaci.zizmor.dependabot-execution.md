# actionsqaci.zizmor.dependabot-execution

## What it forbids or requires

No update in `.github/dependabot.yml` sets `insecure-external-code-execution: allow`. The audit
reads the Dependabot configuration, not a workflow. It runs at zizmor's regular persona, which is
what the lane runs by default, and works offline.

## Hazard

`allow` lets Dependabot run code that comes from the dependency manifests while it works out an
update, such as a build script of a package. That code runs in Dependabot's environment, with
access to what the update job holds, on the strength of whatever a dependency says.

## Correct construction

Set `deny`, or leave the field out, since `deny` is the default:

```yaml
version: 2
updates:
  - package-ecosystem: pip
    directory: /
    schedule:
      interval: daily
    cooldown:
      default-days: 7
    insecure-external-code-execution: deny
```

## Exceptions

An ecosystem whose update cannot be worked out without running its manifest, and whose manifest
the project owns, is recorded in the project record, `.github/actions-qa-ci.yml`, with this
identifier, the file, a `hazard`, a `scope` and a `decision` (see
`actionsqaci.unjustifiedException`). A `# zizmor: ignore` comment is refused by
`actionsqaci.inlineSuppression`.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#dependabot-execution>
