package zizmor

// dependabotPath is the file the two dependabot audits read.
const dependabotPath = ".github/dependabot.yml"

// dependabotOf is a fixture of one Dependabot configuration with these extra lines for its one
// update (already indented four spaces).
func dependabotOf(extras string) fixture {
	return fixture{target: dependabotPath, files: map[string]string{
		dependabotPath: "version: 2\nupdates:\n  - package-ecosystem: pip\n    directory: /\n    schedule:\n      interval: daily\n" + extras,
	}}
}

// cooldown is the cooldown the dependabot audits find enough.
const cooldown = "    cooldown:\n      default-days: 7\n"

// appJob is the job line that lets a step use a secret without secrets-outside-env reporting it,
// and appStep a step that mints a GitHub App token scoped to one repository and one permission.
const (
	appJob  = "    environment: app\n"
	appStep = "      - name: token\n        uses: actions/create-github-app-token@df432ceedc7162793a195dd1713ff69aefc7379e # v2.0.6\n" +
		"        with:\n          app-id: ${{ vars.APP_ID }}\n          private-key: ${{ secrets.APP_KEY }}\n" +
		"          repositories: x\n          permission-contents: read\n"
)

func init() {
	addFixtures(map[string]pair{
		"adhoc-packages": {
			red:       workflowOf("", "      - name: install\n        run: gem install rake\n"),
			green:     workflowOf("", "      - name: install\n        run: bundle install\n"),
			atDefault: true,
		},
		"anonymous-definition": {
			red: single("on: push\npermissions: {}\nconcurrency:\n  group: x-${{ github.ref }}\n  cancel-in-progress: true\n" +
				jobWith("", cleanSteps)),
			green: clean(),
		},
		"archived-uses": {
			red:       workflowOf("", "      - name: release\n        uses: actions/create-release@0cb9c9b65d5d1901c1f53e5e66eaf4afd303e70e # v1.1.4\n"),
			green:     clean(),
			atDefault: true,
		},
		"bot-conditions": {
			red:       workflowOf("    if: github.actor == 'dependabot[bot]'\n", cleanSteps),
			green:     workflowOf("    if: github.event.pull_request.user.login == 'dependabot[bot]'\n", cleanSteps),
			atDefault: true,
		},
		"cache-poisoning": {
			red: single("name: x\non: release\npermissions: {}\nconcurrency:\n  group: x-${{ github.ref }}\n  cancel-in-progress: true\n" +
				jobWith("", cleanSteps+"      - name: cache\n        uses: actions/cache@0057852bfaa89a56745cba8c7296529d2fc39830 # v4.3.0\n        with:\n          path: ~/.cache\n          key: k\n")),
			green:     clean(),
			atDefault: true,
		},
		"dependabot-cooldown": {
			red:       dependabotOf(""),
			green:     dependabotOf(cooldown),
			atDefault: true,
		},
		"dependabot-execution": {
			red:       dependabotOf(cooldown + "    insecure-external-code-execution: allow\n"),
			green:     dependabotOf(cooldown + "    insecure-external-code-execution: deny\n"),
			atDefault: true,
		},
		"github-app": {
			red:       workflowOf(appJob, appStep+"          skip-token-revoke: true\n"),
			green:     workflowOf(appJob, appStep),
			atDefault: true,
		},
		"github-env": {
			// The audit reads only a workflow with a privileged trigger, which dangerous-triggers
			// reports whatever the job does, so that finding cannot be avoided in the red fixture.
			red: single("name: x\non:\n  workflow_run:\n    workflows: [x]\n    types: [completed]\npermissions: {}\nconcurrency:\n  group: x-${{ github.ref }}\n  cancel-in-progress: true\n" +
				jobWith("", "      - name: env\n        run: echo \"X=$X\" >> \"$GITHUB_ENV\"\n        env:\n          X: ${{ github.event.workflow_run.head_branch }}\n")),
			alsoFires: []string{"dangerous-triggers"},
			green:     workflowOf("", "      - name: out\n        id: out\n        run: echo \"x=$X\" >> \"$GITHUB_OUTPUT\"\n        env:\n          X: ${{ github.ref_name }}\n"),
			atDefault: true,
		},
		"insecure-commands": {
			red:       workflowOf("    env:\n      ACTIONS_ALLOW_UNSECURE_COMMANDS: true\n", cleanSteps),
			green:     clean(),
			atDefault: true,
		},
	})
}
