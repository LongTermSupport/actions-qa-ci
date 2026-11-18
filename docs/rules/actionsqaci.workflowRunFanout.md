# actionsqaci.workflowRunFanout

## What it forbids or requires

In a workflow started by `workflow_run` on a completed run (`types` absent or naming
`completed`), every job is gated on the conclusions of the watched run it is for, written as an
allowlist:

- its `if:` compares `github.event.workflow_run.conclusion` with a value (`== 'failure'`, or
  `contains(fromJSON('["failure","timed_out"]'), github.event.workflow_run.conclusion)`), and
  never with `!=` or a negated `contains`;
- or it `needs` a job, and so inherits that job's skip, without overriding it with `always()` or
  `cancelled()`.

One finding is reported at each job that is not gated, so an exception can name the job.

## Hazard

A watched run ends `cancelled` whenever concurrency supersedes it, and `skipped` whenever its own
condition was false. A job that runs on every conclusion, or on a denylist such as
`conclusion != 'success'`, starts a runner for each of those runs: every superseded build on a
busy branch fans out into a watcher job that has nothing to report.

## Correct construction

```yaml
on:
  workflow_run:
    workflows: [CI]
    types: [completed]
    branches: [main]

jobs:
  report:
    if: github.event.workflow_run.conclusion == 'failure'
    runs-on: ubuntu-latest
    steps:
      - run: echo "the run failed"
```

## Exceptions

A job that genuinely has to see every conclusion (one that records each run's outcome, cancelled
ones included) takes an exception for this identifier, the workflow's path and the job, stating
why a cancelled run must start it:

```yaml
exceptions:
  - id: actionsqaci.workflowRunFanout
    path: .github/workflows/run-ledger.yml
    job: record
    hazard: "Every cancelled or skipped CI run starts a ledger job"
    scope: "Only the record job, whose ledger must hold every run's conclusion"
    decision: https://github.com/OWNER/REPO/issues/1
```

## Upstream

First-party. The conclusions a run can end with are listed for the `workflow_run` event in
"Events that trigger workflows" (<https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#workflow_run>).
The condition is read as text, not evaluated (`dbf.json`, `workflow-run-condition-not-evaluated`).
