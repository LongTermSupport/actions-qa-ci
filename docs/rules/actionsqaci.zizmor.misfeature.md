# actionsqaci.zizmor.misfeature

## What it forbids or requires

A workflow uses none of three features GitHub Actions offers but zizmor considers a mistake:

- the `pip-install` input of `actions/setup-python`;
- `shell: cmd`, the Windows command interpreter;
- a shell that is not well known (for example `nushell`). This third check runs only at zizmor's
  auditor persona.

The first two run at the regular persona, which the lane reports by default; the third is shown
only by the rule's own harness (the project record cannot enable part of an audit the lane already
runs). The audit does not use the network.

## Hazard

`pip-install` installs into the runner's own Python with no environment and no lock file, and
`cmd` has quoting and escaping rules that make it easy to turn data into commands. An unusual shell
is a program the step trusts without it being part of the runner's well-known set.

## Correct construction

```yaml
steps:
  - name: set up python
    uses: actions/setup-python@3d3c42e5aac5ba805825da76410c181273ba90b1 # v5.0.0
  - name: install
    run: |
      python -m venv .env
      ./.env/bin/pip install .[dev]
```

Use `bash` or `pwsh` rather than `cmd`.

## Exceptions

Recorded in the project record, `.github/actions-qa-ci.yml`, with this identifier, the file, the
job and step, and a `hazard`, `scope` and `decision` that meet `actionsqaci.unjustifiedException`.
A step that needs a shell other than the well-known ones is the usual case, and the scope names
that step.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#misfeature>
