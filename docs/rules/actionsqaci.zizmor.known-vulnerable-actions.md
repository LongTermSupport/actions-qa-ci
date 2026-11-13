# actionsqaci.zizmor.known-vulnerable-actions

## What it forbids or requires

This build does not run this audit yet. The identifier and this page exist so that a finding of it
is never an unknown one. The audit asks GitHub about the repositories a workflow uses, and this
build runs zizmor offline.

Once it runs, a `uses:` that names an action version with a published advisory in the GitHub
Advisories database, such as credential disclosure or template injection, is reported.

## Hazard

A publicly disclosed flaw in an action is a flaw in every workflow that runs the vulnerable
version, and the advisory tells an attacker how to use it.

## Correct construction

Upgrade to a fixed version, pinned to a full commit SHA, or remove the action:

```yaml
steps:
  - name: checkout
    uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v4.2.0
    with:
      persist-credentials: false
```

## Exceptions

None can be recorded: the audit is not run. Once it is, a vulnerability that cannot apply to the
workflow is recorded in the project record with the advisory, the hazard, the scope and the
decision, held to the standard of `actionsqaci.unjustifiedException`.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#known-vulnerable-actions>
