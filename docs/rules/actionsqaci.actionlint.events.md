# actionsqaci.actionlint.events

## What it forbids or requires

`on:` names real webhook events, and each event's configuration (activity types, filters,
`schedule`) is valid for that event.

## Hazard

A misspelled or unsupported event can never trigger the workflow, so it does not run when the
author expects it to.

## Correct construction

```yaml
on:
  push:
    branches: [main]
  pull_request:
```

## Exceptions

An exception is recorded in the project record with the file, this identifier and a justification
that names the event GitHub added after the pinned actionlint release. The project record is not
part of this build yet, so no exception can be recorded today: use an event the pinned release
knows.

## Upstream

actionlint v1.7.12, kind `events`:
<https://github.com/rhysd/actionlint/blob/v1.7.12/docs/checks.md#webhook-events-validation>
