# actionsqaci.zizmor.archived-uses

## What it forbids or requires

No `uses:` refers to an action or reusable workflow in an archived repository. The audit runs at
zizmor's regular persona, which is what the lane runs by default. It works offline, against a list
of archived actions that zizmor carries, so it reports the ones on that list and does not ask
GitHub about any other.

## Hazard

An archived repository is read-only and unmaintained: a flaw found in it is never fixed, and the
action keeps running with the workflow's token and secrets.

## Correct construction

Remove the action, or replace it with a maintained one. Many archived actions can be replaced by
the `gh` command line tool that the runner image provides:

```yaml
steps:
  - name: release
    run: gh release create "$TAG" --generate-notes
    env:
      GH_TOKEN: ${{ github.token }}
      TAG: ${{ github.ref_name }}
```

## Exceptions

An archived action that is kept on purpose, for instance because it is vendored and reviewed, is
recorded in the project record, `.github/actions-qa-ci.yml`, with this identifier, the file, the
job, a `hazard`, a `scope` and a `decision` (see `actionsqaci.unjustifiedException`). A
`# zizmor: ignore` comment is refused by `actionsqaci.inlineSuppression`.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#archived-uses>
