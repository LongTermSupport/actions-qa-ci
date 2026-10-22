# actionsqaci.actionlint.action

## What it forbids or requires

`uses:` is a well-formed reference, `{owner}/{repo}@{ref}`, `{owner}/{repo}/{path}@{ref}` or a local
path, and the `with:` inputs it passes are inputs the action declares.

## Hazard

A reference without a version fails at run time, and an undeclared input is silently ignored by the
action, so the step does not do what the author believes.

## Correct construction

Pin to a full commit SHA, with the release in a comment:

```yaml
steps:
  - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
    with:
      persist-credentials: false
```

## Exceptions

An exception is recorded in the project record with the file, this identifier and a justification.
The project record is not part of this build yet, so no exception can be recorded today: fix the
reference.

## Upstream

actionlint v1.7.12, kind `action`:
<https://github.com/rhysd/actionlint/blob/v1.7.12/docs/checks.md#action-format-in-uses>
