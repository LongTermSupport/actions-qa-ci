# actionsqaci.requiredChecks

## What it forbids or requires

Every status check the project record requires is the name of a check run that a job of a
workflow triggered on `pull_request` produces. The declaration is the `requiredChecks` section of
`.github/actions-qa-ci.yml`:

```yaml
version: 1
requiredChecks:
  - context: QA Pipeline
    branch: default
```

`branch` is optional and means the repository's default branch; it names one branch, never a
pattern. The same context on two branches is two requirements, and stating one requirement twice
is refused with exit 2.

A requirement is a finding, reported at its entry in the record, when:

- no job in a workflow directly in `.github/workflows` produces the context: the name is stale,
  misspelt, or belongs to a job that was renamed. A job's check run is called its `name:` when it
  has one, and its id otherwise;
- the only jobs that produce it are in workflows not triggered on `pull_request` (a push, a
  schedule or `pull_request_target` never reports to a pull request's merge box);
- the job that carries the name has an expression in it (`${{ … }}`), so the check run is never
  called what the file says, or runs a `matrix`, so each of its check runs carries a suffix and the
  plain name is never one.

A job that calls a reusable workflow (`uses:`) produces contexts named `<its name> / <the called
job>`; a context under that prefix is accepted, as the called workflow is not read here. A
context a matrix job would carry, such as `Tests (8.5)`, is reported with the matrix reason.

A context that an app or a commit status reports (code scanning, a coverage service) is not
produced by any workflow, so it is a finding; accept it with an exception.

A workflow that cannot be read as YAML, has no mapping of jobs, or is a link, a pipe or too
large, is a finding at the workflow itself, since the checks it produces are then unknown. Only files directly in
`.github/workflows` ending `.yml` or `.yaml` are workflows, as GitHub reads them.

Comparing the requirement with the branch protection a repository actually has needs the network
and a token and is not done here (the known gap `required-checks-online-comparison` in `dbf.json`).

## Hazard

A required status check that nothing produces never reports, so a pull request waits for it for
ever or, worse, an administrator learns to merge past it. The usual cause is a rename: the job is
renamed or removed, the branch protection keeps the old name, and the gate silently stops
guarding. A check whose name carries an expression or a matrix suffix cannot be required by
name at all, and one that runs only on push never reaches the pull request.

## Correct construction

Give the job a literal `name:` that is the context, trigger its workflow on `pull_request`, and
keep the record in step when either changes:

```yaml
on:
  pull_request:
jobs:
  qa:
    name: QA Pipeline
    runs-on: ubuntu-latest
```

To require every cell of a matrix, add one job that `needs:` the matrix job and require that
job's name.

## Exceptions

An exception names the record file and accepts a requirement that is deliberately not yet
produced, for example while a workflow is being migrated. It is held to the justification
standard of the record and is reported as stale once a job produces the context.

## Upstream

First-party. It replaces the hand-kept list of contexts in php-qa-ci's
`scripts/setup-branch-protection.bash` (`OUR_CONTEXTS`)
(<https://github.com/LongTermSupport/php-qa-ci/blob/php8.5/scripts/setup-branch-protection.bash>).
