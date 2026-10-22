# actionsqaci.actionlint.permissions

## What it forbids or requires

`permissions:` is `read-all`, `write-all`, an empty mapping, or a mapping of scopes GitHub defines,
each with a value that scope accepts: `read`, `write` or `none`, except that `id-token` takes only
`write` or `none` and `models` only `read` or `none`.

## Hazard

An invalid scope or value makes the workflow invalid, so the job token does not get the access the
author declared.

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
