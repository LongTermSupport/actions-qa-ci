# actionsqaci.zizmor.typosquat-uses

## What it forbids or requires

No `uses:` names a repository whose owner and name are a close misspelling of a well-known action
(`action/checkout` for `actions/checkout`) owned by a different account. zizmor compares each
slug with a list of popular ones. It runs at the regular persona, so the lane reports it by
default. It works offline, at low confidence: with a GitHub token zizmor would also check that the
lookalike resolves to a live repository, and this build runs zizmor with no token and no network.

## Hazard

An action runs with the job's token and secrets. A repository registered under a name one
character away from a trusted action is how a typing mistake, or a deliberately planted
reference, hands a job to someone else's code.

## Correct construction

```yaml
steps:
  - name: checkout
    uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v4.2.0
    with:
      persist-credentials: false
```

## Exceptions

A real repository whose name happens to be close to a popular one can be recorded in the project
record, `.github/actions-qa-ci.yml`, with the file, this identifier, the job and step, and a
hazard, scope and decision that say why the repository is the intended one and who owns it.
Otherwise correct the reference.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#typosquat-uses>
