# actionsqaci.zizmor.cache-poisoning

## What it forbids or requires

A workflow that publishes a release does not read from a cache, and a job that runs on a
privileged trigger (see `actionsqaci.zizmor.dangerous-triggers`) does not write to one. The audit
looks for cache-aware actions, such as `actions/cache` and the `cache` inputs of the setup actions,
and for the `cache-mode` of those that have one. It runs at zizmor's regular persona, which is what
the lane runs by default, and works offline.

## Hazard

A cache entry is shared by every workflow run of the repository that can reach its key. Code that
is less trusted than the release can write an entry, and the release then restores it and builds
or signs from it: the artifact published is not the one the source describes.

## Correct construction

Turn the cache off in a release workflow, at the workflow or the job, or give the action the input
that disables it:

```yaml
on: release
jobs:
  release:
    name: release
    runs-on: ubuntu-latest
    steps:
      - name: build
        run: make release
```

For an action that has a mode, such as `cache-mode: none`, say so on the step.

## Exceptions

A cache that holds nothing the release can execute or ship is recorded in the project record,
`.github/actions-qa-ci.yml`, with this identifier, the file, the job, a `hazard`, a `scope` and a
`decision` (see `actionsqaci.unjustifiedException`). A `# zizmor: ignore` comment is refused by
`actionsqaci.inlineSuppression`.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#cache-poisoning>
