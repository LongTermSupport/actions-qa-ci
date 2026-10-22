# actionsqaci.templateDrift

## What it forbids or requires

Every copy of a template that the project record declares is byte-identical to its source. The
declaration is the `templates` section of `.github/actions-qa-ci.yml`:

```yaml
version: 1
templates:
  - source: templates/github-actions/php-qa-ci.yml
    copies:
      - .github/workflows/qa.yml
```

Three things are findings:

- a copy that differs from its source in any byte, reported at the copy, with the line where the
  two first differ (counted in line feeds, from 1; a line ending written differently is a
  difference on that line);
- a source or a copy that is missing, reported at the record's entry for the template;
- a source or a copy that is not a plain file (a directory, a link, a pipe) or is larger than
  1 MiB, reported at the same place. Such a file is never read.

A missing file is a finding and not silence on purpose: a copy that is renamed or deleted must not
turn its own check off. A path is repository-relative, with forward slashes, and names one file;
a file may not be both a source and a copy, a copy may not be declared twice, and the record is
refused with exit 2 if any of that is wrong.

This build compares whole files. A copy that is allowed to vary in one named YAML key is not
supported yet: a `mode` key in a template is refused as unknown, so a record never states
something that is ignored.

## Hazard

A shipped template and the workflow that dogfoods it drift apart one edit at a time. The copy
that runs in the project's own CI then stops being the thing consumers receive, so a defect the
template carries is not seen by the project that ships it, and one the project fixed in its
copy is never fixed for its consumers. A check that goes quiet when a file is renamed is worse
than none, because the project believes the pair is guarded.

## Correct construction

Edit the source and copy it over the copies, or declare the copies you mean to keep identical:

```sh
cp templates/github-actions/php-qa-ci.yml .github/workflows/qa.yml
```

If a copy has to differ, it is not a copy: take it out of the record and make the difference the
copy's own file, or generate it from the source.

## Exceptions

An exception names the copy's path (or the record's, for a missing file). It accepts a known,
temporary difference and is held to the justification standard of the record; it is reported as
stale once the copy matches again.

## Upstream

First-party. It generalises the identity check of php-qa-ci's `versionPins` tool
(<https://github.com/LongTermSupport/php-qa-ci/blob/php8.5/docs/tools/versionPins.md>), which goes
quiet when the copy is renamed; this check reports a missing file instead.
