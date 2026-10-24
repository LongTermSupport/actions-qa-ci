# actionsqaci.zizmor.unpinned-uses

## What it forbids or requires

Every `uses:` of an action names a full commit SHA. A `uses:` with no ref (the default branch, or
`:latest` for a Docker action) and one with a branch or tag such as `@v4` are reported. The lane
runs zizmor with its default policy, which requires a hash pin for every action, the repository's
own included. A `docker://` reference needs a version suffix such as `:24.04`.

zizmor reports this at the regular persona, which the lane runs by default. The audit works
offline.

## Hazard

GitHub has no immutable branches or tags. The owner of the action, or whoever takes over their
account, can move a tag to different code, and the next run of this workflow executes it with the
job's token and secrets, with nothing in this repository changing.

## Correct construction

```yaml
steps:
  - name: checkout
    uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v4.2.0
    with:
      persist-credentials: false
```

Keep the SHA current with Dependabot or Renovate; the version comment beside it is checked by
`actionsqaci.zizmor.ref-version-mismatch`.

## Exceptions

None. A hash pin costs one line, and zizmor's policy file, which could relax it for a repository
pattern, is refused by `actionsqaci.nativeSuppressionConfig`.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#unpinned-uses>
