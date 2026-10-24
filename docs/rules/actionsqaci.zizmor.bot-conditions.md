# actionsqaci.zizmor.bot-conditions

## What it forbids or requires

A condition does not recognise a bot by `github.actor`, as in
`github.actor == 'dependabot[bot]'`. The audit runs at zizmor's regular persona, which is what the
lane runs by default, and works offline.

## Hazard

`github.actor` is the last actor to act on the trigger, not the author of the change. An attacker
can open a pull request and have the bot act on it, for example by commenting `@dependabot rebase`, so that the check passes for a change the bot did not write. Used to gate an auto-merge
or a privileged step, that is a way past the gate.

## Correct construction

Check who created the pull request, and do not run such a workflow on `pull_request_target`:

```yaml
jobs:
  automerge:
    name: automerge
    runs-on: ubuntu-latest
    if: github.event.pull_request.user.login == 'dependabot[bot]'
```

## Exceptions

A condition that only chooses what to print, and gates nothing, is recorded in the project record,
`.github/actions-qa-ci.yml`, with this identifier, the file, the job, a `hazard`, a `scope` and a
`decision` (see `actionsqaci.unjustifiedException`). A `# zizmor: ignore` comment is refused by
`actionsqaci.inlineSuppression`.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#bot-conditions>
