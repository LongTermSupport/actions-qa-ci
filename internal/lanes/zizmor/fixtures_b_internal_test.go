package zizmor

// stepsAfterCheckout is the clean steps' checkout, then the steps given, so that a fixture adds
// its defect to a workflow that has nothing for any audit.
func stepsAfterCheckout(more string) string {
	return "" +
		"      - name: checkout\n" +
		"        uses: " + checkoutPin + "\n" +
		"        with:\n" +
		"          persist-credentials: false\n" + more
}

func init() {
	addFixtures(map[string]pair{
		"misfeature": {
			red:       workflowOf("", stepsAfterCheckout("      - name: shell\n        shell: cmd\n        run: echo hi\n")),
			green:     clean(),
			atDefault: true,
		},
		"obfuscation": {
			red:       workflowOf("", stepsAfterCheckout("      - name: hi\n        run: echo \"$X\"\n        env:\n          X: ${{ format('{0}/{1}', 'octocat', 'hello-world') }}\n")),
			green:     workflowOf("", stepsAfterCheckout("      - name: hi\n        run: echo \"$X\"\n        env:\n          X: octocat/hello-world\n")),
			atDefault: true,
		},
		"overprovisioned-secrets": {
			red:       workflowOf("    environment: prod\n", stepsAfterCheckout("      - name: hi\n        run: echo \"$ALL\"\n        env:\n          ALL: ${{ toJSON(secrets) }}\n")),
			green:     workflowOf("    environment: prod\n", stepsAfterCheckout("      - name: hi\n        run: echo \"$ONE\"\n        env:\n          ONE: ${{ secrets.ONE }}\n")),
			atDefault: true,
		},
		"secrets-inherit": {
			red:       single(header + "jobs:\n  a:\n    name: a\n    permissions:\n      contents: read\n    uses: x/y/.github/workflows/r.yml@3d3c42e5aac5ba805825da76410c181273ba90b1 # v1.0.0\n    secrets: inherit\n"),
			green:     single(header + "jobs:\n  a:\n    name: a\n    permissions:\n      contents: read\n    uses: x/y/.github/workflows/r.yml@3d3c42e5aac5ba805825da76410c181273ba90b1 # v1.0.0\n    secrets:\n      ONE: ${{ secrets.ONE }}\n"),
			atDefault: true,
		},
		"secrets-outside-env": {
			red:   workflowOf("", stepsAfterCheckout("      - name: hi\n        run: echo \"$ONE\"\n        env:\n          ONE: ${{ secrets.ONE }}\n")),
			green: workflowOf("    environment: prod\n", stepsAfterCheckout("      - name: hi\n        run: echo \"$ONE\"\n        env:\n          ONE: ${{ secrets.ONE }}\n")),
		},
		"self-hosted-runner": {
			red:   single(header + "jobs:\n  a:\n    name: a\n    runs-on: self-hosted\n    permissions:\n      contents: read\n    steps:\n" + cleanSteps),
			green: clean(),
		},
		"self-repository": {
			red:       workflowOf("", stepsAfterCheckout("      - name: local\n        uses: ./my-action\n")),
			green:     workflowOf("", stepsAfterCheckout("      - name: local\n        uses: $/my-action\n")),
			atDefault: true,
		},
	})
}
