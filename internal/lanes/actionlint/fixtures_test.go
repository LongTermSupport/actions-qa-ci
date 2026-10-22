package actionlint_test

// fixture is a small repository: the workflow the lane is pointed at, and every file in it.
type fixture struct {
	target string
	files  map[string]string
}

// pair is a red fixture the rule must fire on and a green one, the corrected construction,
// it must stay quiet on.
type pair struct {
	red   fixture
	green fixture
}

const (
	workflowPath = ".github/workflows/w.yml"
	calleePath   = ".github/workflows/callee.yml"
	checkoutPin  = "actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1"
)

// workflow builds a one-job workflow from the parts a fixture varies.
func workflow(trigger, jobExtras, steps string) fixture {
	source := "name: x\n" + trigger + "jobs:\n  a:\n    runs-on: ubuntu-latest\n" + jobExtras + steps

	return fixture{target: workflowPath, files: map[string]string{workflowPath: source}}
}

const (
	pushTrigger = "on: push\n"
	runHi       = "    steps:\n      - run: echo hi\n"
)

const callee = "name: callee\non:\n  workflow_call:\n    inputs:\n      x:\n        required: true\n        type: string\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hi\n"

func caller(with string) fixture {
	source := "name: caller\non: push\njobs:\n  a:\n    uses: ./" + calleePath + "\n" + with

	return fixture{target: workflowPath, files: map[string]string{workflowPath: source, calleePath: callee}}
}

// fixtures has one pair for every kind of finding actionlint reports as itself.
var fixtures = map[string]pair{
	"syntax-check": {
		red:   workflow(pushTrigger, "    stepz: []\n", runHi),
		green: workflow(pushTrigger, "", runHi),
	},
	"matrix": {
		red:   workflow(pushTrigger, "    strategy:\n      matrix:\n        os: [a, a]\n", runHi),
		green: workflow(pushTrigger, "    strategy:\n      matrix:\n        os: [a, b]\n", runHi),
	},
	"credentials": {
		red: workflow(pushTrigger,
			"    container:\n      image: ghcr.io/x/y:1\n      credentials:\n        username: user\n        password: hunter2\n", runHi),
		green: workflow(pushTrigger,
			"    container:\n      image: ghcr.io/x/y:1\n      credentials:\n        username: user\n        password: ${{ secrets.REGISTRY_PASSWORD }}\n", runHi),
	},
	"shell-name": {
		red:   workflow(pushTrigger, "", "    steps:\n      - run: echo hi\n        shell: bashh\n"),
		green: workflow(pushTrigger, "", "    steps:\n      - run: echo hi\n        shell: bash\n"),
	},
	"runner-label": {
		red:   fixture{target: workflowPath, files: map[string]string{workflowPath: "name: x\non: push\njobs:\n  a:\n    runs-on: ubuntu-lates\n" + runHi}},
		green: workflow(pushTrigger, "", runHi),
	},
	"events": {
		red:   workflow("on: pushh\n", "", runHi),
		green: workflow(pushTrigger, "", runHi),
	},
	"job-needs": {
		red: workflow(pushTrigger, "    needs: [missing]\n", runHi),
		green: fixture{target: workflowPath, files: map[string]string{workflowPath: "name: x\non: push\njobs:\n" +
			"  b:\n    runs-on: ubuntu-latest\n" + runHi +
			"  a:\n    runs-on: ubuntu-latest\n    needs: [b]\n" + runHi}},
	},
	"action": {
		red:   workflow(pushTrigger, "", "    steps:\n      - uses: actions/checkout\n"),
		green: workflow(pushTrigger, "", "    steps:\n      - uses: "+checkoutPin+"\n"),
	},
	"env-var": {
		red:   workflow(pushTrigger, "    env:\n      \"A B\": x\n", runHi),
		green: workflow(pushTrigger, "    env:\n      AB: x\n", runHi),
	},
	"id": {
		red:   fixture{target: workflowPath, files: map[string]string{workflowPath: "name: x\non: push\njobs:\n  1job:\n    runs-on: ubuntu-latest\n" + runHi}},
		green: fixture{target: workflowPath, files: map[string]string{workflowPath: "name: x\non: push\njobs:\n  job1:\n    runs-on: ubuntu-latest\n" + runHi}},
	},
	"glob": {
		red:   workflow("on:\n  push:\n    branches: ['[']\n", "", runHi),
		green: workflow("on:\n  push:\n    branches: [main]\n", "", runHi),
	},
	"permissions": {
		red:   workflow(pushTrigger+"permissions:\n  contents: writee\n", "", runHi),
		green: workflow(pushTrigger+"permissions:\n  contents: read\n", "", runHi),
	},
	"workflow-call": {
		red:   caller(""),
		green: caller("    with:\n      x: hello\n"),
	},
	"expression": {
		red:   workflow(pushTrigger, "", "    steps:\n      - run: echo ${{ github.evnt }}\n"),
		green: workflow(pushTrigger, "", "    steps:\n      - run: echo ${{ github.event_name }}\n"),
	},
	"deprecated-commands": {
		red:   workflow(pushTrigger, "", "    steps:\n      - run: echo \"::set-output name=a::b\"\n"),
		green: workflow(pushTrigger, "", "    steps:\n      - run: echo \"a=b\" >> \"$GITHUB_OUTPUT\"\n"),
	},
	"if-cond": {
		red:   workflow(pushTrigger, "", "    steps:\n      - if: ${{ github.event_name }} == 'push'\n        run: echo hi\n"),
		green: workflow(pushTrigger, "", "    steps:\n      - if: ${{ github.event_name == 'push' }}\n        run: echo hi\n"),
	},
}
