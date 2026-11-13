# actionsqaci.zizmor.insecure-url-scheme

## What it forbids or requires

A URL that zizmor reads from a file it analyses uses a secure scheme. zizmor limits the audit to the
`repo:` field of a pre-commit configuration and flags only the schemes it knows to be insecure, such
as `http` and `git`. It runs at zizmor's regular persona and does not use the network.

This build does not run the audit yet: it analyses workflows, actions and the Dependabot file, and
hands zizmor no pre-commit configuration. The rule keeps its identifier and this page so that a
finding of it is never an unknown one, and running the rule says why it is held.

## Hazard

A repository fetched over `http://` or `git://` is neither authenticated nor protected in transit,
so anyone on the path can substitute the code the hook then runs, in the developer's checkout or on
the runner.

## Correct construction

```yaml
repos:
  - repo: https://github.com/x/y
    rev: v1.0.0
    hooks:
      - id: z
```

## Exceptions

A host that offers nothing but an insecure scheme is not a reason to fetch code from it: mirror the
repository somewhere that serves `https` or `ssh`. If an exception is accepted anyway, it is
recorded in the project record, `.github/actions-qa-ci.yml`, with this identifier, the file, and a
`hazard`, `scope` and `decision` that meet `actionsqaci.unjustifiedException`.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#insecure-url-scheme>
