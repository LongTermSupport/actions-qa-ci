# actionsqaci.zizmor.overprovisioned-secrets

## What it forbids or requires

An expression does not expose the whole `secrets` context, as `${{ toJSON(secrets) }}` does. Each
secret a step needs is named. It runs at zizmor's regular persona, so the lane reports it by
default. It does not use the network.

## Hazard

Every secret the repository, its environments and its organisation make available to the job is
placed in the step's environment, whether the step needs it or not. A compromise of that step
leaks all of them, and the runner can only redact a value that is a secret on its own.

## Correct construction

```yaml
jobs:
  a:
    environment: prod
    steps:
      - name: use one secret
        run: echo "$ONE"
        env:
          ONE: ${{ secrets.ONE }}
```

## Exceptions

Recorded in the project record, `.github/actions-qa-ci.yml`, with this identifier, the file, the
job and step, and a `hazard`, `scope` and `decision` that meet `actionsqaci.unjustifiedException`.
A step that must receive every secret is the case to justify, and the scope names that step.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#overprovisioned-secrets>
