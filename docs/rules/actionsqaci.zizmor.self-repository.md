# actionsqaci.zizmor.self-repository

## What it forbids or requires

A step that uses an action in the repository's own checkout writes it with GitHub's dedicated
self-repository syntax, `uses: $/my-action`, and not the workspace-relative `uses: ./my-action`.
It runs at zizmor's regular persona, so the lane reports it by default. It does not use the
network.

## Hazard

A `./` path is resolved against the runner's workspace at the time the step runs, so it names
whatever has been checked out into that directory by then, including a different ref or an
untrusted pull request's files. The `$/` form names the repository and commit the workflow itself
runs from.

## Correct construction

```yaml
- name: local
  uses: $/my-action
```

## Exceptions

Recorded in the project record, `.github/actions-qa-ci.yml`, with this identifier, the file, the
job and step, and a `hazard`, `scope` and `decision` that meet `actionsqaci.unjustifiedException`.
A runner that does not yet support the `$/` syntax, such as a GitHub Enterprise Server instance, is
the case to justify.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#self-repository>
