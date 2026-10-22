# actionsqaci.actionlint.credentials

## What it forbids or requires

The `credentials` of a job container or service container take their password from a secret, never
from a literal in the workflow file.

## Hazard

A literal password is committed to history and readable by everyone who can read the repository.

## Correct construction

```yaml
container:
  image: ghcr.io/example/image:1
  credentials:
    username: ${{ github.actor }}
    password: ${{ secrets.REGISTRY_PASSWORD }}
```

## Exceptions

There is no reason to put a real password in a workflow. A value that is public by design, such as a
documented read-only token, is recorded in the project record with the file, this identifier and a
justification naming why it is public. The project record is not part of this build yet, so no
exception can be recorded today: use a secret.

## Upstream

actionlint v1.7.12, kind `credentials`:
<https://github.com/rhysd/actionlint/blob/v1.7.12/docs/checks.md#hardcoded-credentials>
