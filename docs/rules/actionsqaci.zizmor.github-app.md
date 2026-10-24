# actionsqaci.zizmor.github-app

## What it forbids or requires

A GitHub App installation token is short-lived and as narrow as the job needs. The audit reports
three uses of the token action: `skip-token-revoke: true`, which stops the token being revoked
when the job ends; `owner:` given without `repositories:`, which makes a token for every
repository of the installation; and no `permission-*` inputs, which gives the token every
permission of the app. It runs at zizmor's regular persona, which is what the lane runs by
default, and works offline.

## Hazard

A token that outlives its job, or reaches more repositories or permissions than the job uses, is
worth more to anyone who captures it, from a log, an artifact or a compromised step.

## Correct construction

```yaml
jobs:
  a:
    name: a
    runs-on: ubuntu-latest
    environment: app
    steps:
      - name: token
        uses: actions/create-github-app-token@df432ceedc7162793a195dd1713ff69aefc7379e # v2.0.6
        with:
          app-id: ${{ vars.APP_ID }}
          private-key: ${{ secrets.APP_KEY }}
          repositories: x
          permission-contents: read
```

The job names an `environment` because the private key is a secret (see
`actionsqaci.zizmor.secrets-outside-env`).

## Exceptions

A token that must outlive the job, or must reach every repository of the installation, is recorded
in the project record, `.github/actions-qa-ci.yml`, with this identifier, the file, the job, a
`hazard`, a `scope` and a `decision` (see `actionsqaci.unjustifiedException`). A
`# zizmor: ignore` comment is refused by `actionsqaci.inlineSuppression`.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#github-app>
