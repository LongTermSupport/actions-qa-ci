# actionsqaci.zizmor.self-hosted-runner

## What it forbids or requires

A job runs on a GitHub-hosted runner. zizmor flags `runs-on` that names a self-hosted runner,
including runner services such as namespace. This audit runs at zizmor's pedantic persona only, so
the lane does not report it by default: the rule's own harness shows it until the project record's
`enable` exists. It does not use the network.

## Hazard

A self-hosted runner is not reset to a known image for each job unless it is configured to be
ephemeral. A job that runs untrusted code, such as a pull request from a fork of a public
repository, can leave behind a process or a file that a later job on the same machine meets, and
the machine sits inside the network that hosts it.

## Correct construction

```yaml
jobs:
  a:
    runs-on: ubuntu-latest
```

Where self-hosting is needed, use it on private repositories, make the runners ephemeral, and
require approval before a fork's workflow runs.

## Exceptions

Self-hosting is a recorded decision. Record it in the project record,
`.github/actions-qa-ci.yml`, with this identifier, the file, the job, and a `hazard`, `scope` and
`decision` that meet `actionsqaci.unjustifiedException`; the hazard states what code can reach the
runner and how it is reset.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#self-hosted-runner>
