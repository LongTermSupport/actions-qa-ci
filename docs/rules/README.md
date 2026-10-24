# Rule identifiers

Generated from the registry; do not edit. Regenerate it with
`go test . -run TestTheIdentifierIndexIsCurrent -update`.

| Identifier | Standing instruction | Page |
| --- | --- | --- |
| `actionsqaci.actionlint.action` | uses: names a well-formed action reference with a version and valid inputs | [page](actionsqaci.actionlint.action.md) |
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
| `actionsqaci.agentSummaryCurrent` | The generated region of CLAUDE.md or AGENTS.md lists the defences this build has and what the project record narrows | [page](actionsqaci.agentSummaryCurrent.md) |
| `actionsqaci.inlineSuppression` | No comment tells zizmor, ratchet or ShellCheck to leave a finding out | [page](actionsqaci.inlineSuppression.md) |
| `actionsqaci.lintCoverage` | Every git-tracked file that is workflow-shaped or action-shaped is analysed, or declared unanalysed with a justification | [page](actionsqaci.lintCoverage.md) |
| `actionsqaci.nativeSuppressionConfig` | No tool's own configuration file suppresses a finding: zizmor's file, an actionlint ignore, a pinact ignore_actions | [page](actionsqaci.nativeSuppressionConfig.md) |
| `actionsqaci.requiredChecks` | Every status check the project record requires is the name of a job in a workflow triggered on pull_request | [page](actionsqaci.requiredChecks.md) |
| `actionsqaci.shellcheck.SC<nnnn>` | Shell in a run: step has no ShellCheck error or warning | [page](actionsqaci.shellcheck.md) |
| `actionsqaci.staleException` | Every exception in the project record matches a finding in a full run | [page](actionsqaci.staleException.md) |
| `actionsqaci.templateDrift` | Each declared copy of a template is byte-identical to its source, and a missing source or copy is a finding | [page](actionsqaci.templateDrift.md) |
| `actionsqaci.unjustifiedException` | Every exception in the project record states a hazard, a scope and a decision, and every unanalysed file a hazard and a scope, none of them generic | [page](actionsqaci.unjustifiedException.md) |
| `actionsqaci.zizmor.adhoc-packages` | No run: step installs a package by name; dependencies come from a lock file | [page](actionsqaci.zizmor.adhoc-packages.md) |
| `actionsqaci.zizmor.anonymous-definition` | Every workflow and action definition has a name: (reported by the rule only: zizmor's regular persona, which check runs, leaves it out) | [page](actionsqaci.zizmor.anonymous-definition.md) |
| `actionsqaci.zizmor.archived-uses` | No uses: refers to an archived repository | [page](actionsqaci.zizmor.archived-uses.md) |
| `actionsqaci.zizmor.artipacked` | Checkout does not leave credentials where an uploaded artifact could carry them out | [page](actionsqaci.zizmor.artipacked.md) |
| `actionsqaci.zizmor.bot-conditions` | A bot is recognised by a check an attacker cannot spoof, not by github.actor | [page](actionsqaci.zizmor.bot-conditions.md) |
| `actionsqaci.zizmor.cache-poisoning` | A release workflow does not read a cache that untrusted code could have written, and a privileged-trigger job does not write one | [page](actionsqaci.zizmor.cache-poisoning.md) |
| `actionsqaci.zizmor.concurrency-limits` | A workflow states a concurrency limit (reported by the rule only: zizmor's regular persona, which check runs, leaves it out) | [page](actionsqaci.zizmor.concurrency-limits.md) |
| `actionsqaci.zizmor.dangerous-triggers` | No workflow runs on a trigger that gives untrusted input a privileged token | [page](actionsqaci.zizmor.dangerous-triggers.md) |
| `actionsqaci.zizmor.dependabot-cooldown` | Dependabot waits out a cooldown before it proposes a new release | [page](actionsqaci.zizmor.dependabot-cooldown.md) |
| `actionsqaci.zizmor.dependabot-execution` | Dependabot does not allow insecure external code execution | [page](actionsqaci.zizmor.dependabot-execution.md) |
| `actionsqaci.zizmor.excessive-permissions` | A workflow's token permissions are the least it needs | [page](actionsqaci.zizmor.excessive-permissions.md) |
| `actionsqaci.zizmor.forbidden-uses` | uses: stays inside the allow or deny list the project sets (not run by this build: it does nothing until it is given a policy, and the project record cannot yet give it one) | [page](actionsqaci.zizmor.forbidden-uses.md) |
| `actionsqaci.zizmor.github-app` | A GitHub App token is scoped to what the job needs and revoked when it ends | [page](actionsqaci.zizmor.github-app.md) |
| `actionsqaci.zizmor.github-env` | No step of a privileged-trigger workflow writes a non-literal value to GITHUB_ENV or GITHUB_PATH | [page](actionsqaci.zizmor.github-env.md) |
| `actionsqaci.zizmor.hardcoded-container-credentials` | Container and service credentials come from secrets, never as literals | [page](actionsqaci.zizmor.hardcoded-container-credentials.md) |
| `actionsqaci.zizmor.impostor-commit` | A pinned commit belongs to the repository it is pinned in, not to a fork of it (not run by this build: it asks GitHub about the repositories a workflow uses, and this build runs zizmor offline) | [page](actionsqaci.zizmor.impostor-commit.md) |
| `actionsqaci.zizmor.insecure-commands` | ACTIONS_ALLOW_UNSECURE_COMMANDS is never set | [page](actionsqaci.zizmor.insecure-commands.md) |
| `actionsqaci.zizmor.insecure-url-scheme` | A pre-commit repo: URL uses https or ssh, not http or git (not run by this build: it reads only a pre-commit configuration, and this build does not collect those) | [page](actionsqaci.zizmor.insecure-url-scheme.md) |
| `actionsqaci.zizmor.known-vulnerable-actions` | No uses: names an action version that has a published advisory (not run by this build: it asks GitHub about the repositories a workflow uses, and this build runs zizmor offline) | [page](actionsqaci.zizmor.known-vulnerable-actions.md) |
| `actionsqaci.zizmor.misfeature` | No pip-install input, cmd shell or non-standard shell | [page](actionsqaci.zizmor.misfeature.md) |
| `actionsqaci.zizmor.obfuscation` | uses: paths and expressions are written plainly, not obfuscated | [page](actionsqaci.zizmor.obfuscation.md) |
| `actionsqaci.zizmor.overprovisioned-secrets` | A step does not expose the whole secrets context | [page](actionsqaci.zizmor.overprovisioned-secrets.md) |
| `actionsqaci.zizmor.ref-confusion` | A uses: ref is not a name that is both a branch and a tag (not run by this build: it asks GitHub about the repositories a workflow uses, and this build runs zizmor offline) | [page](actionsqaci.zizmor.ref-confusion.md) |
| `actionsqaci.zizmor.ref-version-mismatch` | The version comment beside a pinned commit names that commit's release (not run by this build: it asks GitHub about the repositories a workflow uses, and this build runs zizmor offline) | [page](actionsqaci.zizmor.ref-version-mismatch.md) |
| `actionsqaci.zizmor.secrets-inherit` | A reusable workflow is given the secrets it needs, not secrets: inherit | [page](actionsqaci.zizmor.secrets-inherit.md) |
| `actionsqaci.zizmor.secrets-outside-env` | A secret is used only in a job that has a dedicated environment (reported by the rule only: zizmor's regular persona, which check runs, leaves it out) | [page](actionsqaci.zizmor.secrets-outside-env.md) |
| `actionsqaci.zizmor.self-hosted-runner` | A job runs on a GitHub-hosted runner unless self-hosting is a recorded decision (reported by the rule only: zizmor's regular persona, which check runs, leaves it out) | [page](actionsqaci.zizmor.self-hosted-runner.md) |
| `actionsqaci.zizmor.self-repository` | A step that uses an action of the repository's own checkout is written uses: $/path, not uses: ./path | [page](actionsqaci.zizmor.self-repository.md) |
| `actionsqaci.zizmor.stale-action-refs` | A pinned commit is the commit of a release tag (not run by this build: it asks GitHub about the repositories a workflow uses, and this build runs zizmor offline) | [page](actionsqaci.zizmor.stale-action-refs.md) |
| `actionsqaci.zizmor.superfluous-actions` | No action does what the runner image already provides (reported by the rule only: zizmor's regular persona, which check runs, leaves it out) | [page](actionsqaci.zizmor.superfluous-actions.md) |
| `actionsqaci.zizmor.template-injection` | No ${{ }} expansion puts attacker-controllable data into code | [page](actionsqaci.zizmor.template-injection.md) |
| `actionsqaci.zizmor.typosquat-uses` | No uses: names a repository that imitates a popular one | [page](actionsqaci.zizmor.typosquat-uses.md) |
| `actionsqaci.zizmor.undocumented-permissions` | Every entry of a permissions: block says why it is needed (reported by the rule only: zizmor's regular persona, which check runs, leaves it out) | [page](actionsqaci.zizmor.undocumented-permissions.md) |
| `actionsqaci.zizmor.unpinned-images` | A container image is named by a digest, not by no tag, latest or a mutable tag | [page](actionsqaci.zizmor.unpinned-images.md) |
| `actionsqaci.zizmor.unpinned-tools` | The known tool-downloading actions (setup-trivy, load-secrets-action) are given a version, not none or latest | [page](actionsqaci.zizmor.unpinned-tools.md) |
| `actionsqaci.zizmor.unpinned-uses` | uses: is pinned to a full commit SHA | [page](actionsqaci.zizmor.unpinned-uses.md) |
| `actionsqaci.zizmor.unredacted-secrets` | A secret is used whole, not as a field of structured data the runner would not redact | [page](actionsqaci.zizmor.unredacted-secrets.md) |
| `actionsqaci.zizmor.unsound-condition` | An if: condition does not hold a fenced expression in a block scalar that keeps its newline, which makes it always true | [page](actionsqaci.zizmor.unsound-condition.md) |
| `actionsqaci.zizmor.unsound-contains` | contains() searches a list, not a string a partial match could satisfy | [page](actionsqaci.zizmor.unsound-contains.md) |
| `actionsqaci.zizmor.unsound-ternary` | A && / \|\| pseudo-ternary does not have a statically falsy true value | [page](actionsqaci.zizmor.unsound-ternary.md) |
| `actionsqaci.zizmor.use-trusted-publishing` | A packaging workflow publishes with trusted publishing, not a long-lived token | [page](actionsqaci.zizmor.use-trusted-publishing.md) |
