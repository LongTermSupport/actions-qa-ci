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
the project record's `docsOnly.paths` adds, less those a `docsOnly.inputs` entry names. A `push`
filtered to tags alone is not checked, since a branch push never starts it and GitHub applies no
path filter to a tag.

The record narrows the check in two ways only, and each entry states its reason:

- `docsOnly.inputs` names, by path or pattern, a documentation file that is a real input of the
  build or the tests: a README a test reads, pages embedded in the program. A file it names is not
  documentation to the check, so a workflow's filter may take it back in.
- `docsOnly.exempt` names a workflow in `.github/workflows` that must run for a docs-only change,
  such as a scan of what a push publishes. That workflow is not checked.

A reason is held to the standard of an exception's hazard: missing, generic, a pointer to another
entry, or under 40 characters or 6 words is an `actionsqaci.unjustifiedException` finding. The
entries are refused, and the record with them, when a list is written empty, a pattern cannot be
used, an entry is listed twice, an input names a file `docsOnly.paths` adds, inputs take out every
file the check would probe, or an exempt entry is not a `.yml` or `.yaml` file directly in
`.github/workflows`. With the record, the check reports at the entry an input that matches no
tracked file, and an exempt entry that names no tracked workflow or one a docs-only change does
not start: an entry that names nothing would narrow the check for a file added later without
anyone deciding.

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
release is cut from) re-includes those paths in the same way (`- "README.md"` after the
`"!**.md"` line, for a README a test reads), and declares them as inputs. When a
docs-only change also touches other files (plan folders, say), or pull requests should skip
documentation too, the record adds them; a workflow that must see every push is exempt:

```yaml
version: 1
docsOnly:
  paths: [CLAUDE/Plan/00001-example/notes.txt]
  events: [pull_request]
  inputs:
    - path: README.md
      reason: The usage test reads the README examples, so a change to them can break the build
  exempt:
    - workflow: .github/workflows/public-scan.yml
      reason: The public-safety scan has to see every file a docs-only push publishes
```

`paths-ignore: ["**.md", "docs/**"]` is enough when nothing needs re-including.

## Exceptions

A workflow that must run on every push is listed under `docsOnly.exempt` with its reason. An
exception for this identifier and the workflow's path is accepted too, and states a hazard, a
scope and a decision.

## Upstream

First-party. The pattern language is GitHub's ("Filter pattern cheat sheet" in the workflow syntax
reference, <https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#filter-pattern-cheat-sheet>),
implemented in `internal/ghfilter`.
