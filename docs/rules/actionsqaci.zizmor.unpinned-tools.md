# actionsqaci.zizmor.unpinned-tools

## What it forbids or requires

A step that uses an action which downloads a tool gives that action a version. zizmor knows a
short list of such actions (`aquasecurity/setup-trivy` and `1password/load-secrets-action`) and
reports a `uses:` of one when `with.version` is missing or is `latest`. An action that pins the
tools it fetches itself is not reported, and neither is an action zizmor does not know. It runs
at the regular persona, so the lane reports it by default, and needs no network.

## Hazard

The action is pinned to a commit, but the tool it fetches is not: `latest` resolves at run time,
so a new release of the tool, or a compromised one, runs in the job with its token and secrets
without any change to the repository.

## Correct construction

```yaml
steps:
  - name: trivy
    uses: aquasecurity/setup-trivy@3d3c42e5aac5ba805825da76410c181273ba90b1 # v0.2.0
    with:
      version: v0.57.1
```

## Exceptions

Tracking the latest release of a tool on purpose is a decision to record: put it in the project
record, `.github/actions-qa-ci.yml`, with the file, this identifier, the job and step, and a
hazard, scope and decision that say why the version cannot be fixed.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#unpinned-tools>
