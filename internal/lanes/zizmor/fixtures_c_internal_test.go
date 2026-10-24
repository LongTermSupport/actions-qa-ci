package zizmor

const (
	// Each of these is a full commit SHA with a version comment beside it.
	rustToolchainPin = "dtolnay/rust-toolchain@086dfa4efe372cfb6b375460a56e26a62a873d2e # 1.93.1"
	setupTrivyPin    = "aquasecurity/setup-trivy@3d3c42e5aac5ba805825da76410c181273ba90b1 # v0.2.0"
	pypiPublishPin   = "pypa/gh-action-pypi-publish@3d3c42e5aac5ba805825da76410c181273ba90b1 # v1.12.0"
)

func init() {
	addFixtures(map[string]pair{
		"superfluous-actions": {
			red:   workflowOf("", cleanSteps+"      - name: rust\n        uses: "+rustToolchainPin+"\n"),
			green: workflowOf("", cleanSteps+"      - name: rust\n        run: rustup toolchain install 1.93.1\n"),
		},
		"typosquat-uses": {
			red: workflowOf("", "      - name: checkout\n        uses: action/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v4.2.0\n"+
				"        with:\n          persist-credentials: false\n"),
			green:     clean(),
			atDefault: true,
		},
		"undocumented-permissions": {
			red: single(header + "jobs:\n  a:\n    name: a\n    runs-on: ubuntu-latest\n    permissions:\n      contents: read\n      packages: read\n    steps:\n" +
				cleanSteps),
			green: single(header + "jobs:\n  a:\n    name: a\n    runs-on: ubuntu-latest\n    permissions:\n      contents: read\n      packages: read # reads the metadata of the package\n    steps:\n" +
				cleanSteps),
		},
		"unpinned-images": {
			red:       workflowOf("    container:\n      image: ghcr.io/x/y\n", cleanSteps),
			green:     workflowOf("    container:\n      image: "+pinnedImage+"\n", cleanSteps),
			atDefault: true,
		},
		"unpinned-tools": {
			red:       workflowOf("", cleanSteps+"      - name: trivy\n        uses: "+setupTrivyPin+"\n"),
			green:     workflowOf("", cleanSteps+"      - name: trivy\n        uses: "+setupTrivyPin+"\n        with:\n          version: v0.57.1\n"),
			atDefault: true,
		},
		"unredacted-secrets": {
			red:       workflowOf("    environment: registry\n", cleanSteps+"      - name: use\n        run: echo \"$PASSWORD\"\n        env:\n          PASSWORD: ${{ fromJSON(secrets.MY_SECRET).password }}\n"),
			green:     workflowOf("    environment: registry\n", cleanSteps+"      - name: use\n        run: echo \"$PASSWORD\"\n        env:\n          PASSWORD: ${{ secrets.MY_SECRET_PASSWORD }}\n"),
			atDefault: true,
		},
		"unsound-condition": {
			red:       workflowOf("", cleanSteps+"      - name: maybe\n        if: |\n          ${{ github.ref == 'refs/heads/main' }}\n        run: echo hi\n"),
			green:     workflowOf("", cleanSteps+"      - name: maybe\n        if: |-\n          ${{ github.ref == 'refs/heads/main' }}\n        run: echo hi\n"),
			atDefault: true,
		},
		"unsound-contains": {
			red:       workflowOf("", cleanSteps+"      - name: maybe\n        if: contains('refs/heads/main refs/heads/develop', github.ref)\n        run: echo hi\n"),
			green:     workflowOf("", cleanSteps+"      - name: maybe\n        if: contains(fromJSON('[\"refs/heads/main\", \"refs/heads/develop\"]'), github.ref)\n        run: echo hi\n"),
			atDefault: true,
		},
		"unsound-ternary": {
			red:       workflowOf("", cleanSteps+"      - name: pick\n        run: echo \"$VALUE\"\n        env:\n          VALUE: ${{ github.ref_name && '' || 'bar' }}\n"),
			green:     workflowOf("", cleanSteps+"      - name: pick\n        run: echo \"$VALUE\"\n        env:\n          VALUE: ${{ case(github.ref_name, '', 'bar') }}\n"),
			atDefault: true,
		},
		"use-trusted-publishing": {
			// The documented form with a password: input is not used here, because zizmor gives it two
			// primary locations and the lane refuses a finding with more than one.
			red: workflowOf("", cleanSteps+"      - name: publish\n        run: twine upload dist/*\n"),
			green: single(header + "jobs:\n  a:\n    name: a\n    runs-on: ubuntu-latest\n    environment: pypi\n    permissions:\n      contents: read\n      id-token: write # exchanges the job's identity for a short-lived PyPI token\n    steps:\n" +
				cleanSteps + "      - name: publish\n        uses: " + pypiPublishPin + "\n"),
			atDefault: true,
		},
	})
}
