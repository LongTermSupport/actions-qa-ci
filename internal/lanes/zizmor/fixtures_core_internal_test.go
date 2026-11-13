package zizmor

func init() {
	addFixtures(map[string]pair{
		"template-injection": {
			red:       workflowOf("", "      - name: hi\n        run: echo ${{ github.event.pull_request.title }}\n"),
			green:     workflowOf("    env:\n      TITLE: ${{ github.event.pull_request.title }}\n", "      - name: hi\n        run: echo \"$TITLE\"\n"),
			atDefault: true,
		},
		"artipacked": {
			red:       workflowOf("", "      - name: checkout\n        uses: "+checkoutPin+"\n"),
			green:     clean(),
			atDefault: true,
		},
		"excessive-permissions": {
			red: single("name: x\non: push\nconcurrency:\n  group: x-${{ github.ref }}\n  cancel-in-progress: true\n" +
				"jobs:\n  a:\n    name: a\n    runs-on: ubuntu-latest\n    steps:\n" + cleanSteps),
			green:     clean(),
			atDefault: true,
		},
		"dangerous-triggers": {
			red: single("name: x\non: pull_request_target\npermissions: {}\nconcurrency:\n  group: x-${{ github.ref }}\n  cancel-in-progress: true\n" +
				jobWith("", cleanSteps)),
			green:     clean(),
			atDefault: true,
		},
		"unpinned-uses": {
			red:       workflowOf("", "      - name: checkout\n        uses: actions/checkout@v4\n        with:\n          persist-credentials: false\n"),
			green:     clean(),
			atDefault: true,
		},
		"hardcoded-container-credentials": {
			red: workflowOf("    container:\n      image: "+pinnedImage+"\n      credentials:\n        username: user\n        password: hunter2\n",
				cleanSteps),
			green: workflowOf("    environment: registry\n    container:\n      image: "+pinnedImage+"\n      credentials:\n        username: user\n        password: ${{ secrets.REGISTRY_PASSWORD }}\n",
				cleanSteps),
			atDefault: true,
		},
		"concurrency-limits": {
			red:   single("name: x\non: push\npermissions: {}\n" + jobWith("", cleanSteps)),
			green: clean(),
		},
	})
}
