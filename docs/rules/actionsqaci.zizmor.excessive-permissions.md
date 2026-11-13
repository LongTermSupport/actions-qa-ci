# actionsqaci.zizmor.excessive-permissions

## What it forbids or requires

A workflow's `GITHUB_TOKEN` permissions are the least each job needs: `permissions: {}` at the top
of the workflow, and a `permissions:` block on each job that grants only the scopes that job uses.
A workflow that sets none, and so takes the default, which depends on the organisation's and the
repository's settings, is reported, as is a grant broader than the job needs.

zizmor reports this at the regular persona, which the lane runs by default. The audit works
offline.

## Hazard

Every job inherits the permissions set at the top of the workflow, and an unset block inherits a
default that may include write access. A compromised step then holds a token that can write to
the repository, packages or issues far beyond what the job does.

## Correct construction

```yaml
name: x
on: push
permissions: {}
jobs:
  a:
    name: a
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      - name: hi
        run: echo hi
```

## Exceptions

A scope a job truly needs, such as `contents: write` to publish a release, is granted on that job
alone. A finding to keep is recorded in the project record with the file, this identifier, the
job, the hazard, the scope and the decision, held to the standard of
`actionsqaci.unjustifiedException`.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#excessive-permissions>
