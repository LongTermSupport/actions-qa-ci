# actionsqaci.zizmor.forbidden-uses

## What it forbids or requires

This build does not run this audit yet. The identifier and this page exist so that a finding of it
is never an unknown one. The audit does nothing until it is given a policy, an allow list or a deny
list of the repositories a `uses:` may name, and the project record cannot yet give it one.

Once it runs, a `uses:` of a repository outside the allow list, or inside the deny list, that the
project record sets is reported. The audit works offline; it does not cover `docker://`
references, impostor commits or indirect use. It is opt-in in zizmor, so it reports nothing
without its policy.

## Hazard

A workflow can run any action it names. Without a list, nothing stops a change from adding an
action the project has not reviewed, and with it that action's access to the job's token and
secrets.

## Correct construction

Name only actions the project's list allows; the list will be a setting of the project record.
Today every `uses:` is pinned to a full commit SHA, which `actionsqaci.zizmor.unpinned-uses`
checks:

```yaml
steps:
  - name: checkout
    uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v4.2.0
    with:
      persist-credentials: false
```

## Exceptions

None can be recorded: the audit is not run. Once it is, an action outside the list is either
added to the list, which is a decision of the project record, or removed.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#forbidden-uses>
