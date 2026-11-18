# actionsqaci.docsOnlyPush

## What it forbids or requires

A workflow triggered on `push` starts no run for a push that changes only documentation. Its
`push` trigger has a `paths` or `paths-ignore` filter, and that filter, read the way GitHub reads
it, leaves out every file of a docs-only change.

The check does not compare patterns. It takes the files a docs-only change touches and asks, for
each one alone, whether GitHub would start the workflow: `paths` is read in order, a pattern
starting with `!` takes back what earlier ones matched and the last match decides; `paths-ignore`
leaves a file out when any of its patterns matches. One finding is reported for each trigger that
lets such a file through, at the trigger, naming the files.

The files are `README.md`, `notes/design.md`, `docs/guide/install.md` and
`docs/images/diagram.png` (Markdown at the root and deeper, and anything under `docs/`), and any
the project record's `docsOnly.paths` adds. The record can add files and events and never remove
the defaults: a workflow that should run for documentation is an exception, with its reason. A `push` filtered to tags alone is not checked, since a
branch push never starts it and GitHub applies no path filter to a tag.

Only `push` is checked unless the record's `docsOnly.events` adds `pull_request`. A path filter on
a `pull_request` workflow whose job is a required status check leaves a docs-only pull request
waiting for a check that never reports, so filtering pull requests is the project's decision.

## Hazard

Every push of a README fix, a design note or a plan document runs the whole pipeline: runner
minutes, a queue slot other changes wait behind, and on a shared self-hosted pool, capacity every
other repository needs. Nothing about the code changed, so the run proves nothing new.

## Correct construction

Leave documentation out and keep in the Markdown that changes how agents and tools behave, which
is code in all but extension:

```yaml
on:
  push:
    branches: [main]
    paths:
      - "**"
      - "!**.md"
      - "!docs/**"
      - "**/CLAUDE.md"
      - "**/AGENTS.md"
      - ".claude/**"
      - ".github/**"
```

A project whose documentation is part of the build (pages embedded in a program, a changelog a
release is cut from) re-includes those paths in the same way. When a docs-only change also
touches other files (plan folders, say), or pull requests should skip documentation too, the
record adds them:

```yaml
version: 1
docsOnly:
  paths: [CLAUDE/Plan/00001-example/notes.txt]
  events: [pull_request]
```

`paths-ignore: ["**.md", "docs/**"]` is enough when nothing needs re-including.

## Exceptions

A workflow that must run on every push, a deployment that publishes the documentation itself
say, takes an exception for this identifier and the workflow's path, stating why a docs-only push
has to run it.

## Upstream

First-party. The pattern language is GitHub's ("Filter pattern cheat sheet" in the workflow syntax
reference, <https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#filter-pattern-cheat-sheet>),
implemented in `internal/ghfilter`.
