# actionsqaci.zizmor.hardcoded-container-credentials

## What it forbids or requires

The `credentials` of a job `container` or of a `services` entry take their password from a secret,
never from a literal in the workflow file. zizmor reports a Docker registry password written out
in those blocks.

zizmor reports this at the regular persona, which the lane runs by default. The audit works
offline.

## Hazard

A literal password is committed to history and readable by everyone who can read the repository.

This rule owns the hazard class of a literal container credential. actionlint's `credentials`
check reports the same hazard; a later change makes zizmor the only reporter.

## Correct construction

```yaml
jobs:
  a:
    name: a
    runs-on: ubuntu-latest
    permissions:
      contents: read
    environment: registry
    container:
      image: ghcr.io/x/y@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
      credentials:
        username: user
        password: ${{ secrets.REGISTRY_PASSWORD }}
    steps:
      - name: hi
        run: echo hi
```

The `environment:` is what keeps `actionsqaci.zizmor.secrets-outside-env` quiet.

## Exceptions

There is no reason to put a real password in a workflow. A value that is public by design, such as
a documented read-only token, is recorded in the project record with the file, this identifier,
the hazard, the scope and the decision, the scope naming why it is public, held to the standard
of `actionsqaci.unjustifiedException`. Use a secret in preference.

## Upstream

zizmor, the release the manifest pins:
<https://docs.zizmor.sh/audits/#hardcoded-container-credentials>
