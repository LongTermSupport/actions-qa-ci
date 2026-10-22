# Rule identifiers

Generated from the registry; do not edit. Regenerate it with
`go test . -run TestTheIdentifierIndexIsCurrent -update`.

| Identifier | Standing instruction | Page |
| --- | --- | --- |
| `actionsqaci.actionlint.action` | uses: names a well-formed action reference with a version and valid inputs | [page](actionsqaci.actionlint.action.md) |
| `actionsqaci.actionlint.credentials` | Container and service credentials come from secrets, never as literals | [page](actionsqaci.actionlint.credentials.md) |
| `actionsqaci.actionlint.deprecated-commands` | No set-output, save-state, set-env or add-path workflow command | [page](actionsqaci.actionlint.deprecated-commands.md) |
| `actionsqaci.actionlint.env-var` | Environment variable names contain no space, = or & | [page](actionsqaci.actionlint.env-var.md) |
| `actionsqaci.actionlint.events` | on: names real webhook events with valid event configuration | [page](actionsqaci.actionlint.events.md) |
| `actionsqaci.actionlint.expression` | Every ${{ }} expression parses and type-checks, and none puts untrusted input into a script | [page](actionsqaci.actionlint.expression.md) |
| `actionsqaci.actionlint.glob` | Branch, tag and path filters are valid glob patterns | [page](actionsqaci.actionlint.glob.md) |
| `actionsqaci.actionlint.id` | Job and step IDs are unique and well formed | [page](actionsqaci.actionlint.id.md) |
| `actionsqaci.actionlint.if-cond` | An if: condition is not always true or always false | [page](actionsqaci.actionlint.if-cond.md) |
| `actionsqaci.actionlint.job-needs` | needs: names jobs that exist and form no cycle | [page](actionsqaci.actionlint.job-needs.md) |
| `actionsqaci.actionlint.matrix` | A matrix has no duplicate values and no combination that matches nothing | [page](actionsqaci.actionlint.matrix.md) |
| `actionsqaci.actionlint.permissions` | permissions: uses real scopes with read, write or none | [page](actionsqaci.actionlint.permissions.md) |
| `actionsqaci.actionlint.runner-label` | runs-on names a runner label that exists | [page](actionsqaci.actionlint.runner-label.md) |
| `actionsqaci.actionlint.shell-name` | A step's shell is one GitHub provides | [page](actionsqaci.actionlint.shell-name.md) |
| `actionsqaci.actionlint.syntax-check` | A workflow file is valid GitHub Actions workflow syntax | [page](actionsqaci.actionlint.syntax-check.md) |
| `actionsqaci.actionlint.workflow-call` | A reusable-workflow call passes the inputs and secrets the callee declares | [page](actionsqaci.actionlint.workflow-call.md) |
| `actionsqaci.shellcheck.SC<nnnn>` | Shell in a run: step has no ShellCheck error or warning | [page](actionsqaci.shellcheck.md) |
| `actionsqaci.staleException` | Every exception in the project record matches a finding in a full run | [page](actionsqaci.staleException.md) |
| `actionsqaci.unjustifiedException` | Every exception in the project record states a hazard, a scope and a decision, none of them generic | [page](actionsqaci.unjustifiedException.md) |
