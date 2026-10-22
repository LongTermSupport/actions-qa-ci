# actionsqaci.agentSummaryCurrent

## What it forbids or requires

The generated region of the file an agent reads says what this build and the project record say.

`actions-qa rules --write-agent-summary <file>` puts the list of active defences, with what the
project record narrows beside each, between the lines `<!-- actions-qa-ci:begin -->` and
`<!-- actions-qa-ci:end -->` of `CLAUDE.md` or `AGENTS.md`. For each of those two files at the
repository root that has such a region, the region has to be the one the command would write now.
A finding is reported at the begin marker of a region that differs, and at the top of a file whose
markers do not form exactly one region (a begin without an end, two regions, an end before the
begin).

A file with no region is the project's own and is not looked at: a project that does not use the
summary is not asked to. A file that is absent, or is a symbolic link (the file behind a link is
judged under its own name), is not compared. A file larger than 1 MiB cannot be compared, and is
exit 4 rather than a clean result. The comparison ignores the file's line endings, and the region
is the same whether the project has a record or not.

## Hazard

An agent reads the summary as the standing rules of the project. A summary written before a
defence was added, removed or narrowed tells the agent a rule that is no longer true: it spends
effort on a defence that is gone, or does not know about one that now blocks its work, or is told
a defence is on that the record turned off.

## Correct construction

Run the command the finding names and commit the result:

```sh
actions-qa rules --write-agent-summary CLAUDE.md
```

Keep the generated region as the command writes it; text inside the markers is replaced on the
next run. Anything the project wants to say about its own defences goes outside them.

## Exceptions

An exception names the file the finding is at. A region that is out of date has one correct
content, so accepting it stale is accepting that an agent is misinformed; the exception needs the
justification the record requires for that.

## Upstream

First-party. The toolchain specification asks for the active defences to be listed for an agent
and for the listing to be checked for currency; php-qa-ci's `rules --write-agent-summary` is the
shape this follows (<https://github.com/LongTermSupport/php-qa-ci>).
