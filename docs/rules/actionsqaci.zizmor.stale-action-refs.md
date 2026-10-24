# actionsqaci.zizmor.stale-action-refs

## What it forbids or requires

This build does not run this audit yet. The identifier and this page exist so that a finding of it
is never an unknown one. The audit asks GitHub about the repositories a workflow uses, and this
build runs zizmor offline.

Once it runs, a `uses:` pinned to a commit that no Git tag points to is reported. It is a
pedantic audit, shown by the rule's own harness until the project record's `enable` exists.

## Hazard

A commit with no tag may hold bugs the owner never released, and a reader cannot tell which
release the workflow runs, which makes a change of pin hard to review.

## Correct construction

Pin to the commit of a release tag and name the tag in the comment beside it:

```yaml
steps:
  - name: checkout
    uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v4.2.0
    with:
      persist-credentials: false
```

## Exceptions

None can be recorded: the audit is not run. Once it is, a repository that releases from a rolling
branch has no tag to point at, which is a decision to record in the project record with the file,
this identifier, the hazard, the scope and the decision, held to the standard of
`actionsqaci.unjustifiedException`.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#stale-action-refs>
