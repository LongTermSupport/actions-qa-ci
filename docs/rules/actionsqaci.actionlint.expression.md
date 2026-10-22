# actionsqaci.actionlint.expression

## What it forbids or requires

Every `${{ }}` expression parses, refers only to contexts, properties and functions that exist where
it is written, and uses operands of types the operator accepts. An expression in a script must not
interpolate a context an outside contributor controls (an issue or pull request title, a branch
name and the like); actionlint reports that as "potentially untrusted" under this identifier.

## Hazard

An expression that does not type-check is wrong at run time, so a condition or a value is not what
the author wrote. Untrusted input interpolated into a `run:` script is code injection: the value is
pasted into the script before the shell sees it.

## Correct construction

```yaml
steps:
  - run: echo "$EVENT"
    env:
      EVENT: ${{ github.event_name }}
```

## Exceptions

An exception is recorded in the project record with the file, this identifier and a justification.
The project record is not part of this build yet, so no exception can be recorded today: correct the
expression, and pass untrusted input to a script through `env:` as shown above.

## Upstream

actionlint v1.7.12, kind `expression`:
<https://github.com/rhysd/actionlint/blob/v1.7.12/docs/checks.md>
