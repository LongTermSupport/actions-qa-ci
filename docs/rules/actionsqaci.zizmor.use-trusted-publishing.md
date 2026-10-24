# actionsqaci.zizmor.use-trusted-publishing

## What it forbids or requires

A workflow that publishes a package to an index that supports trusted publishing (such as PyPI or
RubyGems) authenticates with it, and does not use a long-lived API token. zizmor reports a
publishing action given a token or password input, and a `run:` step that calls a publishing
command such as `twine upload`, `gem push`, `npm publish` or `cargo publish`. Trusted publishing
is configured once on the package index; the job then exchanges its OIDC identity for a
short-lived credential (`id-token: write`), with the ecosystem's publishing action, for example
`pypa/gh-action-pypi-publish` for PyPI. This audit applies to workflows. It runs at the regular
persona, so the lane reports it by default, and needs no network.

## Hazard

A manually configured token is a long-lived secret that works from anywhere. If it leaks, or the
workflow is subverted, it lets an attacker publish a release in the project's name long after the
run is over. A trusted-publishing credential is minted per run and expires within minutes.

## Correct construction

```yaml
jobs:
  a:
    runs-on: ubuntu-latest
    environment: pypi
    permissions:
      contents: read
      id-token: write # exchanges the job's identity for a short-lived PyPI token
    steps:
      - name: publish
        uses: pypa/gh-action-pypi-publish@3d3c42e5aac5ba805825da76410c181273ba90b1 # v1.12.0
```

## Exceptions

An index that has no trusted publishing cannot follow this. Record that in the project record,
`.github/actions-qa-ci.yml`, with the file, this identifier, the job and step, and a hazard, scope
and decision that name the index and where the token is kept.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#use-trusted-publishing>
