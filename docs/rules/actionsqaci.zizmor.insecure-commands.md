# actionsqaci.zizmor.insecure-commands

## What it forbids or requires

`ACTIONS_ALLOW_UNSECURE_COMMANDS` is not set, at the workflow, the job or the step. It opts back in
to the deprecated workflow commands `set-env` and `add-path`. The audit runs at zizmor's regular
persona, which is what the lane runs by default, and works offline.

## Hazard

The commands were disabled because they let anything a step prints, including output derived from
untrusted text, set an environment variable or extend the search path of the later steps.

## Correct construction

Use the environment files instead of the commands, and do not set the variable:

```yaml
steps:
  - name: setup my-bin
    run: echo "$HOME/.local/my-bin" >> "$GITHUB_PATH"
```

## Exceptions

There is no reason to turn the commands back on in a workflow that can be changed. A workflow that
runs an old action that cannot be replaced is recorded in the project record,
`.github/actions-qa-ci.yml`, with this identifier, the file, the job, a `hazard`, a `scope` and a
`decision` (see `actionsqaci.unjustifiedException`); see also
`actionsqaci.actionlint.deprecated-commands`. A `# zizmor: ignore` comment is refused by
`actionsqaci.inlineSuppression`.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#insecure-commands>
