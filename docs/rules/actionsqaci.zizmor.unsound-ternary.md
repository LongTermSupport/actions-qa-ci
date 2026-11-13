# actionsqaci.zizmor.unsound-ternary

## What it forbids or requires

An expression does not use the `condition && value || fallback` pseudo-ternary when `value` is
statically falsy (`''`, `0`, `false` or `null`). Such an expression always yields the fallback,
because `&&` returns the falsy value and `||` then moves on. Use `case()`:
`${{ case(foo, '', 'bar') }}`. It runs at the regular persona, so the lane reports it by default,
and needs no network.

## Hazard

The expression reads as "if the condition, use the value", and never does. A setting meant to be
empty or off under some condition is silently the fallback every time, which can turn a guard or
a flag the wrong way round.

## Correct construction

```yaml
steps:
  - name: pick
    run: echo "$VALUE"
    env:
      VALUE: ${{ case(github.ref_name, '', 'bar') }}
```

## Exceptions

None: `case()` says the same thing correctly. If one is recorded anyway, it goes in the project
record, `.github/actions-qa-ci.yml`, with the file, this identifier, the job and step, and a
hazard, scope and decision.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#unsound-ternary>
