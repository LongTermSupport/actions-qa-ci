# actionsqaci.zizmor.superfluous-actions

## What it forbids or requires

A workflow does not use an action whose work the runner image already does with a tool it ships.
zizmor keeps a list of such actions (for example the Rust toolchain installer, where `rustup` is
already on the runner, or the release actions, where `gh release create` is) and flags each
`uses:` that names one. This is a pedantic-persona audit: the lane's default run does not report
it, so `actions-qa check` reports it only where the project record's `calibrations.enable` names
this identifier, and the rule's own harness always shows it. It needs no network.

## Hazard

Using the action is not in itself a vulnerability. But an action brings its own dependencies and
its own code into a job that holds a token, so each one widens the attack surface for no gain
when the runner can do the same work with a command it already has.

## Correct construction

```yaml
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - name: rust
        run: rustup toolchain install 1.93.1
```

## Exceptions

An action that does more than the tool it resembles (caching, version resolution, a pinned and
reviewed behaviour) can be kept. Record it in the project record, `.github/actions-qa-ci.yml`,
with the file, this identifier, the job and step, and a hazard, scope and decision that name what
the action adds over the runner's own tool.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#superfluous-actions>
