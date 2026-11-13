# actionsqaci.zizmor.unredacted-secrets

## What it forbids or requires

A secret is used as a whole value, never as a field of structured data. An expression that parses
a secret as JSON and reads a part of it, such as `fromJSON(secrets.MY_SECRET).password`, is
reported. Each field is stored as a secret of its own. It runs at the regular persona, so the lane
reports it by default, and needs no network.

## Hazard

The runner redacts a secret from the job log by matching its exact value. A field extracted from
a JSON secret is a different string, so it is not matched and prints in clear if any step logs it.

## Correct construction

```yaml
jobs:
  a:
    runs-on: ubuntu-latest
    environment: registry
    steps:
      - name: use
        run: echo "$PASSWORD"
        env:
          PASSWORD: ${{ secrets.MY_SECRET_PASSWORD }}
```

## Exceptions

None that this build records for the secret itself: split the secret. An exception may be
recorded in the project record, `.github/actions-qa-ci.yml`, with the file, this identifier, the
job and step, and a hazard, scope and decision, when the structured secret comes from outside the
repository and cannot be split.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#unredacted-secrets>
