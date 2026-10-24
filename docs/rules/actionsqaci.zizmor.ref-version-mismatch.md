# actionsqaci.zizmor.ref-version-mismatch

## What it forbids or requires

A `uses:` clause pinned to a commit hash carries a version comment that names the release that
commit belongs to. zizmor flags a comment that does not match the commit, and, at its pedantic
persona only, a hash pin with no version comment at all.

This build does not run the audit yet. zizmor skips it unless it is given a GitHub API token,
because it asks GitHub which release a commit belongs to, and this build runs zizmor offline with
none of the caller's environment. The rule keeps its identifier and this page so that a finding of it is never
an unknown one, and running the rule says why it is held.

## Hazard

A version comment that disagrees with the pinned commit tells a reviewer, and every tool that
updates pins from the comment, that the code is a release it is not. The pin is then trusted for a
version it does not hold.

## Correct construction

```yaml
- name: checkout
  uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v4.2.0
  with:
    persist-credentials: false
```

The comment names the release of exactly that commit.

## Exceptions

None can be recorded while the audit is held, because it reports nothing. Once it runs, an
exception is recorded in the project record, `.github/actions-qa-ci.yml`, with a `hazard`, `scope`
and `decision` that meet `actionsqaci.unjustifiedException`.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#ref-version-mismatch>
