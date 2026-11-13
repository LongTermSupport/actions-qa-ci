# actionsqaci.nativeSuppressionConfig

## What it forbids or requires

No configuration file of a bundled tool suppresses a finding. Three tools are covered, at the
paths each looks for its file:

- **zizmor**: `zizmor.yml`, `zizmor.yaml`, `.github/zizmor.yml` and `.github/zizmor.yaml`. The
  file is forbidden whatever it holds, because zizmor discovers it by itself and its ignore and
  disable settings are honoured whenever zizmor is run directly.
- **actionlint**: `.github/actionlint.yaml` and `.github/actionlint.yml`, when any mapping in
  them has an `ignore` key (`paths: <glob>: ignore:`). The file's `self-hosted-runner` and
  `config-variables` settings are calibrations, not suppressions, and are allowed.
- **pinact**: `.pinact.yaml`, `.pinact.yml`, `.github/pinact.yaml` and `.github/pinact.yml`, when
  any mapping in them has an `ignore_actions` key. Its other settings are allowed.

One finding is reported for a zizmor file, at its first line, and one for each forbidden key, at
the key. A file that is not readable as YAML cannot be judged and stops the run with a failure of
the toolchain: it is never reported as clean.

## Hazard

These files are suppressions in a place the project record cannot see. They carry no reason, they
cannot be reported when they stop matching anything, and actionlint's `ignore` is a regular
expression over the message text, so one entry can quietly cover findings it was never written
for. The bundled actionlint run never reads its file (it passes an empty configuration of its
own), which makes it worse: the file looks as if it works, and a person running the tool by hand
sees something different from the gate. The bundled zizmor run is given no configuration either
(`--no-config`), with the same result. pinact is not bundled yet; the check forbids its files now
so that the day it is, no suppression is waiting for it.

## Correct construction

Delete the file, or the forbidden key. A calibration goes in the project record, and a finding
that is accepted becomes an exception there:

```yaml
version: 1
exceptions:
  - id: actionsqaci.actionlint.expression
    path: .github/workflows/matrix.yml
    job: build
    hazard: "A matrix expression the expression checker cannot type because the matrix is generated"
    scope: "Only the build job, whose matrix comes from one reviewed JSON file in the repository"
    decision: https://github.com/LongTermSupport/php-qa-ci/issues/123
```

## Exceptions

None are useful: an exception covers the file it names, and the file is the problem. Delete it.

## Upstream

First-party, from the Defence Before Fix toolchain specification's requirement that a defence
forbid every suppression route that bypasses the project record. The files and keys are those of
zizmor (<https://docs.zizmor.sh/configuration/>), actionlint
(<https://github.com/rhysd/actionlint/blob/main/docs/config.md>) and pinact
(<https://github.com/suzuki-shunsuke/pinact>).
