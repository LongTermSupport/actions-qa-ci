# actionsqaci.lintCoverage

## What it forbids or requires

Every file the repository tracks that looks like a workflow or like a composite action is either
analysed by this toolchain or declared, in the project record, not to be.

- A file is **workflow-shaped** when it is a `.yml` or `.yaml` file whose top level has an `on`
  key and a `jobs` mapping.
- A file is **action-shaped** when it is named `action.yml` or `action.yaml` and its top level has a
  `runs` key.

The files analysed by default (see `check`) are the workflows directly in `.github/workflows` and
the actions at the root and under `.github/actions`. Any other file of either shape is a finding at
the file, on the line of its `jobs` or `runs` key. That includes a workflow in a subdirectory of
`.github/workflows`, which GitHub never reads. A file the project means to leave alone is declared
in `.github/actions-qa-ci.yml`:

```yaml
version: 1
analyse:
  unanalysed:
    - path: tests/fixtures/**
      justification:
        hazard: "Fixture workflows hold the very mistakes the rules are tested against"
        scope: "Only the fixtures directory, whose files no workflow of the project ever runs"
```

`path` is a pattern anchored at the repository root. In a segment `*` matches any run of
characters and `?` any one, neither crossing a slash; a segment that is exactly `**` matches any
number of whole segments (none included in the middle of a pattern, at least one at the end, so
`tests/**` names what is inside `tests`). Character classes, braces, negation, a backslash, a
leading or trailing slash and a `.` or `..` segment are refused when the record is read. The
`hazard` and `scope` are held to the standard of `actionsqaci.unjustifiedException`.

A declaration that excuses no file is itself a finding, at its entry in the record: one that names
nothing (a typo, a directory since removed) would excuse a file added later without anyone
deciding, and one that names only files analysed anyway turns nothing off. A declaration never
removes a file of the default set from the analysis.

The check reads the repository's own index, so it covers the files `git` tracks and no others: a
file that is not yet added is not seen, and a link, a submodule or a file deleted from the work
tree has nothing to look at. A file over 1 MiB, or that is not valid YAML, is not examined and
counts as neither shape. An index this build cannot read completely (a split or sparse index, one
of a repository that does not use SHA-1) is exit 3 and not a clean result.

## Hazard

A workflow or action that no lane analyses is checked by nothing: its injections, its unpinned
`uses:` and its excessive permissions go unreported, and a template that consumers copy ships with
them. The survey that began this toolchain found a directory of shipped templates that was never
scanned, for exactly this reason: scanning starts from where GitHub looks, and a file elsewhere
simply is not there.

## Correct construction

Put the workflow where GitHub reads it, `.github/workflows`, or the action under `.github/actions`
or at the root, so that it is analysed. If the file is deliberately outside the analysis, declare
it with the hazard it would be checked for and the scope that makes leaving it unchecked
acceptable.

## Exceptions

An exception names the file the finding is at. A finding about a declaration is at the project
record, and an exception there is accepted only with the justification the record requires. A
declaration in `analyse.unanalysed` is the way to accept a whole group of files, and an exception
the way to accept one.

## Upstream

First-party. The design takes it from the survey of php-qa-ci's own workflows, in which the files
under `templates/github-actions/` were found not to be analysed by anything
(<https://github.com/LongTermSupport/php-qa-ci>).
