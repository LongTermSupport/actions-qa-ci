# actionsqaci.actionlint.permissions

## What it forbids or requires

`permissions:` names scopes GitHub defines and gives each `read`, `write` or `none`.

## Hazard

An invalid scope or value makes the workflow invalid, or leaves the job token with different access
from the one intended.

## Correct construction

```yaml
permissions:
  contents: read
```

## Exceptions

An exception is recorded in the project record with the file, this identifier and a justification.
The project record is not part of this build yet, so no exception can be recorded today: use a
scope and a value GitHub defines. This rule checks that the declaration is valid; whether it is
narrow enough is a separate rule's concern.

## Upstream

actionlint v1.7.12, kind `permissions`:
<https://github.com/rhysd/actionlint/blob/v1.7.12/docs/checks.md#permissions>
