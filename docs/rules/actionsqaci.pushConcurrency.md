# actionsqaci.pushConcurrency

## What it forbids or requires

A workflow that a push to `main` starts has a `concurrency` group, at the top of the workflow or
on every job, and the group is not built from a value that differs in every run (`github.sha`,
`github.run_id`, `github.run_number`, `github.run_attempt`, `github.event.after`,
`github.event.head_commit.id`).

A push trigger counts when its `branches` filter admits `main` (read in order, last match wins),
its `branches-ignore` does not leave `main` out, or it has neither. A push filtered to tags alone
does not count. One finding is reported per workflow: at the push trigger when there is no
`concurrency`, at `concurrency` when it has no group or a per-run one.

`cancel-in-progress` is not judged for a group without a per-run value. Whether a superseded run
on `main` is cancelled, or finishes while GitHub keeps only the newest pending run behind it, is
the repository's decision.

One shape with a per-run value is accepted: a run per push, a shared group per pull request. The
group's per-run value sits only in the second arm of
`github.event_name == 'pull_request' && <shared value> || <per-run value>`, the first arm and the
rest of the group hold no per-run value, and `cancel-in-progress` is exactly
`${{ github.event_name == 'pull_request' }}`. A pull request's runs then share a group and cancel
each other, while every run of any other event, a push to `main` among them, has its own group
and runs to the end. The shape states in the workflow that every commit on `main` is built,
which is what an exception would otherwise state. A group written as a plain string, a
`cancel-in-progress` of anything else, a condition on another event or on `!=`, or a per-run
value anywhere outside that arm is judged as above. The expression is matched as text with its
whitespace removed, not evaluated (`dbf.json`, `push-concurrency-condition-not-evaluated`).

## Hazard

Without a shared group, every push to `main` runs to the end however many pushes follow it.
A burst of merges queues one full run per commit, each one superseded before it starts, and each
one holds a runner the next pull request waits for. A group keyed on the commit or the run id is
the same thing written longer: no two runs ever share it.

## Correct construction

```yaml
concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true
```

To keep a running `main` build and drop only the queued ones, say so:

```yaml
concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: ${{ github.event_name == 'pull_request' }}
```

To build every commit on `main` while a pull request's runs still collapse, use the accepted
per-run shape:

```yaml
concurrency:
  group: ${{ github.workflow }}-${{ github.event_name == 'pull_request' && format('pr-{0}', github.event.pull_request.number) || github.run_id }}
  cancel-in-progress: ${{ github.event_name == 'pull_request' }}
```

## Exceptions

A workflow whose every push must be built separately writes the per-run shape above. One that
needs a per-run group in any other form takes an exception for this identifier and the
workflow's path, stating why a superseded run still has to finish.

## Upstream

First-party. GitHub's concurrency semantics are in "Control the concurrency of workflows and jobs"
(<https://docs.github.com/en/actions/how-tos/write-workflows/choose-when-workflows-run/control-workflow-concurrency>).
The default branch is taken to be `main` (`dbf.json`, `push-concurrency-default-branch-is-main`).
