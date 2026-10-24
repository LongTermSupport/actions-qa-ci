# actionsqaci.zizmor.obfuscation

## What it forbids or requires

A `uses:` path and an expression are written plainly. zizmor flags a `uses:` path with redundant
`/` separators or with `.` or `..` segments, and an expression that computes what it could state,
such as `fromJSON(toJSON(...))` or `format(...)` whose arguments are all literals. It runs at
zizmor's regular persona, so the lane reports it by default. It does not use the network.

## Hazard

An obfuscated reference hides from a reader, and from other audits and tools that match on the
text, what a workflow really uses or runs. A reviewer cannot tell by looking that the action or
value is what it appears to be.

## Correct construction

```yaml
env:
  REPOSITORY: octocat/hello-world
```

Not `${{ format('{0}/{1}', 'octocat', 'hello-world') }}`.

## Exceptions

Recorded in the project record, `.github/actions-qa-ci.yml`, with this identifier, the file, the
job and step, and a `hazard`, `scope` and `decision` that meet `actionsqaci.unjustifiedException`.
There is rarely a reason: the plain form is shorter.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#obfuscation>
