# actionsqaci.zizmor.undocumented-permissions

## What it forbids or requires

Each entry of an explicit `permissions:` block carries a comment saying why the permission is
needed. An entry without one is reported. `contents: read` is not reported, since nearly every job
needs it. This is a pedantic-persona audit about maintainability: the lane's default run does not
report it, so `actions-qa check` reports it only where the project record's `calibrations.enable`
names this identifier, and the rule's own harness always shows it. It needs no network.

## Hazard

A permission nobody can explain is a permission nobody dares remove. Without the reason beside
it, a reviewer cannot tell whether a broad grant is still needed, so token scopes only grow.

## Correct construction

```yaml
permissions:
  contents: read
  packages: read # reads the metadata of the package
```

## Exceptions

Write the comment rather than record an exception. If one is still wanted, record it in the
project record, `.github/actions-qa-ci.yml`, with the file, this identifier, the job, and a
hazard, scope and decision that say why this block goes unexplained.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#undocumented-permissions>
