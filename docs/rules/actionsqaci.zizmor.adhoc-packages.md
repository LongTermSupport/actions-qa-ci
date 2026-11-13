# actionsqaci.zizmor.adhoc-packages

## What it forbids or requires

A `run:` step does not install or change packages outside a locked manifest. zizmor recognises the
npm, yarn and pnpm commands of JavaScript and the gem and bundle commands of Ruby, such as
`gem install rake`, `bundle add` and `yarn add`. The audit runs at zizmor's regular persona, which
is what the lane runs by default, and works offline.

## Hazard

A package named in the command is resolved to whatever version the registry offers on the day the
job runs. Nothing in the repository fixes that version or its dependencies, so a hijacked or
newly published release is installed into a job that holds the workflow's token and secrets.

## Correct construction

Put the package in a manifest that produces a lock file, commit the lock file, and install from it
with a lock-aware command:

```yaml
steps:
  - name: install
    run: bundle install
```

## Exceptions

A tool that exists only to be installed by name, with no manifest to hold it, is recorded in the
project record, `.github/actions-qa-ci.yml`, with this identifier, the file, the job, a `hazard`, a
`scope` and a `decision` (see `actionsqaci.unjustifiedException`). A `# zizmor: ignore` comment is
refused by `actionsqaci.inlineSuppression`.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#adhoc-packages>
