# actionsqaci.staleException

## What it forbids or requires

Every exception in the project record, `.github/actions-qa-ci.yml`, matches at least one finding
in a full run: the identifier and the path it names (or, with no path, the identifier anywhere),
and the job and step if it names them. An exception that matches nothing is reported at its own
line in the record, so it can be deleted.

It is judged only against a full run, every lane over every file the project has. A run of one
lane, or of named files, reports no stale exception, because the finding that would keep it may
be in a part that was not run.

## Hazard

A record that is never pruned keeps accepting what is no longer there. When the code an exception
was written for is fixed or deleted, the exception stays, and it silently covers the next
finding that happens to have the same identifier and path: a hazard nobody decided to accept is
accepted for them.

## Correct construction

Delete the exception. If the code it covered still exists but no longer draws the finding, the
defence has done its work and the entry has nothing left to say. If the finding is expected and
is not being drawn, the exception names the wrong identifier, path, job or step: correct the
entry so it matches.

```yaml
version: 1
exceptions: []
```

## Exceptions

None. This check cannot itself be excepted, and neither can `actionsqaci.unjustifiedException`:
a record whose judge could be silenced would be one nobody could judge.

## Upstream

First-party. It is one of the three checks of the justification lane that the project-record
requirement of the Defence Before Fix toolchain specification (6.2) calls for, with
`actionsqaci.unjustifiedException`.
