# actionsqaci.zizmor.artipacked

## What it forbids or requires

A step that uses `actions/checkout` states `persist-credentials: false`, or states
`persist-credentials: true` where a later step of the job needs `git` to push. Leaving the input
at its default is reported, because the default leaves the job's token in the checked-out
repository (versions of the action before v6.0.0 write it to `.git/config`).

zizmor reports this at the regular persona, which the lane runs by default. The audit works
offline.

## Hazard

A later step can publish the checked-out directory, for example in an uploaded artifact, and the
persisted credential leaves with it. Whoever downloads the artifact holds a token for the
repository.

## Correct construction

```yaml
steps:
  - name: checkout
    uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v4.2.0
    with:
      persist-credentials: false
```

## Exceptions

A job that pushes with the checked-out credential sets `persist-credentials: true` explicitly,
which states the decision and which zizmor accepts. Any finding that remains is recorded in the
project record with the file, this identifier, the hazard, the scope and the decision, held to the
standard of `actionsqaci.unjustifiedException`.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#artipacked>
