# actionsqaci.scheduleBudget

## What it forbids or requires

A workflow's `schedule` starts no more runs a day than the budget. The budget is the project
record's `scheduleBudget.maxRunsPerDay`, from 1 to 24, and hourly (24) when the record does not
state one.

Each `cron` is read as five fields and counted as the runs it starts on a day it runs at all:
the minutes it selects times the hours. The crons of one workflow are added together. One finding
is reported at the first cron when the total is over the budget, and one at each cron that cannot
be read (a word such as `@hourly`, a value out of range), whose cadence is then unknown.

## Hazard

A schedule more frequent than the work it does needs spends a runner on every tick, all day,
whether anything changed or not: `*/5 * * * *` is 288 runs a day. On a shared pool those runs
queue ahead of the pull requests people are waiting on, and a scheduled job is also the one that
keeps running unwatched after its reason has gone.

## Correct construction

Run as often as the answer is needed, off the top of the hour that every other schedule uses:

```yaml
on:
  schedule:
    - cron: "41 */6 * * *"
  workflow_dispatch:
```

A project that wants a tighter budget states it:

```yaml
version: 1
scheduleBudget:
  maxRunsPerDay: 4
```

## Exceptions

A workflow that must run more often than hourly (a short-lived watch during an incident, say)
takes an exception for this identifier and the workflow's path, stating why and for how long. The
budget itself cannot be raised above hourly, so every such workflow carries its own reason.

## Upstream

First-party. The cron syntax is POSIX cron as GitHub documents it for the `schedule` event
(<https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#schedule>).
