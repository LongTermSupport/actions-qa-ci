# actionsqaci.zizmor.impostor-commit

## What it forbids or requires

This build does not run this audit yet. The identifier and this page exist so that a finding of it
is never an unknown one. The audit asks GitHub about the repositories a workflow uses, and this
build runs zizmor offline.

Once it runs, a `uses:` pinned to a commit that exists in the fork network of the repository but
not on the repository it is written under is reported.

## Hazard

GitHub resolves a commit through a repository's whole fork network, so
`owner/action@<sha>` can name a commit that only a fork holds. A hash pin then hides code the
owner never published, and a reader who sees a full SHA takes it for the owner's.

## Correct construction

Pin to a commit that is on the repository named, and review any change that adds or alters a pin
to confirm that it is:

```yaml
steps:
  - name: checkout
    uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v4.2.0
    with:
      persist-credentials: false
```

## Exceptions

None can be recorded: the audit is not run. Once it is, the fix is to replace the pin with an
authentic commit or ref.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#impostor-commit>
