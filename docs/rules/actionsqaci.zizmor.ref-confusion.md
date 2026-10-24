# actionsqaci.zizmor.ref-confusion

## What it forbids or requires

This build does not run this audit yet. The identifier and this page exist so that a finding of it
is never an unknown one. The audit asks GitHub about the repositories a workflow uses, and this
build runs zizmor offline.

Once it runs, a `uses:` pinned to a symbolic ref, a branch or a tag such as `@v1`, is reported when
the repository holds both a branch and a tag of that name, so that GitHub's choice between them
decides what runs.

## Hazard

An attacker who can create a branch or a tag with the same name as the one a workflow uses can
make GitHub resolve the ref to the attacker's code, which then reaches every consumer of the
workflow.

## Correct construction

Pin to a full commit SHA, which no branch or tag can shadow:

```yaml
steps:
  - name: checkout
    uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v4.2.0
    with:
      persist-credentials: false
```

## Exceptions

None can be recorded: the audit is not run. A hash pin, which `actionsqaci.zizmor.unpinned-uses`
already requires, leaves it nothing to report.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#ref-confusion>
