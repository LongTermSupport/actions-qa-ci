# actionsqaci.unjustifiedException

## What it forbids or requires

Every exception in the project record, `.github/actions-qa-ci.yml`, states three things, and none
of them is a stand-in:

- `hazard`: what the defence would report at that place.
- `scope`: where the exception applies, and why it is accepted only there.
- `decision`: where the acceptance was made, either an `https://` URL (an issue, a pull request:
  one address with a dotted host name or an IP address, no space or control character, no
  credentials) or `Owner:` followed by what was decided (at least one letter or digit).

A `hazard` or `scope` is refused when it is missing, when it is one of the phrases that fit any
entry (`needed for now`, `for now`, `legacy`, `legacy code`, `todo`, `fixme`, `wip`, `temporary`,
`temp`, `will fix later`, `fix later`, `too noisy`, `noisy`, `known issue`, `ignore`,
`false positive`), when it points at another entry (a sentence that opens, or the text that ends, with `see above`,
`as above`, `same as above`, `as before`, `same as before`, `see below` or `ditto`), when every
word of it is a word of the identifier, when it is shorter than 40 characters or has fewer than
6 words (white space counted once), or, for the `scope`, when it is the same sentence as the
`hazard` (case and punctuation do not make it a different one).

An exception with no `path` disables its defence everywhere and is held to the same standard.

The check cannot tell whether a sentence is true. It refuses the sentences that could be written
without knowing anything, so that a reviewer is left with something to judge. Today the record is
read and checked, and an exception in it is not yet applied to any finding.

## Hazard

An exception whose reason fits every entry is a suppression nobody can review: a reader cannot
tell what the defence would have reported there or why that was judged acceptable, so the entry
can never be retired.

## Correct construction

```yaml
version: 1
exceptions:
  - id: actionsqaci.actionlint.glob
    path: .github/workflows/release.yml
    job: release
    hazard: "A branch filter with a character class that the glob matcher reads differently from GitHub's"
    scope: "Only the release job, whose filter is generated and checked against GitHub's behaviour by hand"
    decision: https://github.com/LongTermSupport/php-qa-ci/issues/123
```

If you cannot write the `hazard` and `scope` sentences, the entry is a suppression and the code
is what to change.

## Exceptions

None. An entry that excepts this check has to meet the standard the check enforces, so the way
to clear a finding here is to write the justification.

## Upstream

First-party. It follows the convention of php-qa-ci's check on its `ignoreErrors` entries
(<https://github.com/LongTermSupport/php-qa-ci/blob/php8.5/docs/tools/phpstanIgnoreJustification.md>)
and the project-record requirement of the Defence Before Fix toolchain specification.
