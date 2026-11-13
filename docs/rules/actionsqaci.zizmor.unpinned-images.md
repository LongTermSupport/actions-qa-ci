# actionsqaci.zizmor.unpinned-images

## What it forbids or requires

A job or service `container.image` names its image by digest. Two cases are reported:

- an image with no tag, or the `latest` tag, which the lane reports by default (regular persona);
- an image with any other tag that is not pinned to a `sha256` digest, which only the pedantic
  persona reports, so only the rule's own harness shows it until the project record enables it.

It needs no network.

## Hazard

A tag is a name the registry's owner can move. An image named by tag, and `latest` above all,
can change between two runs of the same commit, so the job runs code nobody reviewed.

## Correct construction

```yaml
jobs:
  a:
    runs-on: ubuntu-latest
    container:
      image: ghcr.io/x/y@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
```

## Exceptions

An image that is rebuilt on purpose and must be taken as it is can be recorded in the project
record, `.github/actions-qa-ci.yml`, with the file, this identifier, the job, and a hazard, scope
and decision that name who publishes the tag and why a digest is not workable.

## Upstream

zizmor, the release the manifest pins: <https://docs.zizmor.sh/audits/#unpinned-images>
