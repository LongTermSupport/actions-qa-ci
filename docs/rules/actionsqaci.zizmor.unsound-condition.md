# actionsqaci.zizmor.unsound-condition

## What it forbids or requires

An `if:` condition that holds a fenced `${{ }}` expression does not sit in a YAML block scalar
that keeps its trailing newline. `if: |` followed by `${{ expr }}` makes the whole condition the
string `"<result>\n"` and not the expression, and a non-empty string is always true. The `|-`
form, or the expression written bare, is correct. It runs at the regular persona, so the lane
reports it by default, and needs no network.

## Hazard

The condition looks like a guard and is not one: the step runs whatever the expression evaluates
to, including on the branch, event or actor it was written to exclude.

## Correct construction

```yaml
steps:
  - name: maybe
    if: |-
      ${{ github.ref == 'refs/heads/main' }}
    run: echo hi
```

## Exceptions

None. A condition that is meant to be always true is written as no condition. If an exception is
still recorded, it goes in the project record, `.github/actions-qa-ci.yml`, with the file, this
identifier, the job and step, and a hazard, scope and decision.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#unsound-condition>
