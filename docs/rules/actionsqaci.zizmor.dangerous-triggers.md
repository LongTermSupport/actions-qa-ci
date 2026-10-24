# actionsqaci.zizmor.dangerous-triggers

## What it forbids or requires

A workflow is not triggered by `pull_request_target`, `workflow_run` or `issue_comment`. Use
`pull_request` in place of `pull_request_target`, and `workflow_call` in place of `workflow_run`.

zizmor reports this at the regular persona, which the lane runs by default. The audit works
offline.

## Hazard

These triggers run in the context of the target repository, with its secrets and a token that can
write, yet a fork's pull request or a stranger's comment can start them. Not running the pull
request's code is not enough to make them safe: a later step, a cache or an artifact can bring the
attacker's data into the privileged job.

## Correct construction

```yaml
name: x
on: push
permissions: {}
concurrency:
  group: x-${{ github.ref }}
  cancel-in-progress: true
jobs:
  a:
    name: a
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      - name: hi
        run: echo hi
```

A trigger that carries no untrusted input to a privileged token (`push`, `pull_request`,
`workflow_call`) is what the audit accepts.

## Exceptions

A workflow that has to react to a fork or a comment, such as a labeller that never checks out or
runs the pull request, is recorded in the project record with the file, this identifier, the
hazard, the scope and the decision, held to the standard of `actionsqaci.unjustifiedException`.
The scope states that no step runs code or reads files from the pull request.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#dangerous-triggers>
