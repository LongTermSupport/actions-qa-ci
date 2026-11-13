# actionsqaci.zizmor.github-env

## What it forbids or requires

A step does not write to `GITHUB_ENV` or `GITHUB_PATH` in a workflow with a privileged trigger,
such as `workflow_run` or `pull_request_target` (see `actionsqaci.zizmor.dangerous-triggers`). A
write of a literal value is not reported; a write of a value taken from the environment or an
expression is. The audit is aimed at workflows with such a trigger and reports nothing for a plain
`push` or `pull_request`. It runs at zizmor's regular persona, which is what the lane runs by
default, and works offline.

## Hazard

A value written to `GITHUB_ENV` becomes an environment variable of every later step, so an
attacker who shapes it can set `LD_PRELOAD`, `NODE_OPTIONS` or a similar variable and run code
there. A value written to `GITHUB_PATH` puts a directory first on the search path of every later
step, which lets a planted program stand in for a real one.

## Correct construction

Pass state between steps through `GITHUB_OUTPUT`, and have the later step read the output
explicitly. Better still, do not run such a step in a workflow that has a privileged trigger:

```yaml
steps:
  - name: out
    id: out
    run: echo "x=$X" >> "$GITHUB_OUTPUT"
    env:
      X: ${{ github.ref_name }}
```

## Exceptions

A write whose value the workflow itself fixes, though not as a literal, is recorded in the project
record, `.github/actions-qa-ci.yml`, with this identifier, the file, the job, a `hazard`, a `scope`
and a `decision` (see `actionsqaci.unjustifiedException`). A `# zizmor: ignore` comment is refused
by `actionsqaci.inlineSuppression`.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#github-env>
