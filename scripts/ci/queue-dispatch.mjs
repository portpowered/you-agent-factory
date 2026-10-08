import assert from "node:assert/strict";
import { REQUIRED_ARTIFACTS } from "./merge-queue-evidence.mjs";
import { selectLintInputs } from "./backend-lint-workflow.mjs";

// Workflow composition is checked in the static lane, separately from the
// component tests of policy, artifact forwarding, and dependency warming.
export function validateQueueDispatch(workflow, { requiredArtifacts = REQUIRED_ARTIFACTS, selectInputs = selectLintInputs } = {}) {
	workflow = workflow.replaceAll("\r\n", "\n");
	const job = (name) => {
		const match = workflow.match(new RegExp(`\\n  ${name}:\\n([\\s\\S]*?)(?=\\n  [a-z0-9_-]+:\\n|$)`));
		if (!match) throw new Error(`Dispatch is missing ${name}.`);
		return match[1];
	};
	const require = (text, expected, label) => {
		if (!text.includes(expected)) throw new Error(`Dispatch must retain ${label}.`);
	};
	// Check the owning job and individual steps, never a main re-upload or a
	// mention in a comment. Exact supported guards fail closed on new expressions.
	const producers = {
		"unit-coverage-diagnostics": { job: "backend-coverage", suite: "unit",
			produce: "Run backend coverage", command: "make test-unit-coverage", path: ".artifacts/unit-coverage/coverage.out" },
		"functional-test-diagnostics": { job: "backend-coverage", suite: "functional",
			produce: "Run Linux functional coverage with concurrent quarantine verification",
			command: "run: bash scripts/ci/run-functional-coverage-with-quarantine.sh", path: ".artifacts/functional-test-viz/coverage.out" },
		"backend-deadcode-evidence": { job: "backend-lint",
			produce: "Run complete canonical Backend Lint inventory",
			command: 'make LINT_REPORT_FILE="$LINT_REPORT_FILE" lint', path: "bin/deadcode-current.txt" },
	};
	for (const artifact of requiredArtifacts) {
		const fail = (detail) => { throw new Error(`Queue artifact ${artifact}: ${detail}`); };
		const producer = producers[artifact];
		if (!producer) fail("no mapped producer");
		let owner;
		try { owner = job(producer.job); } catch { fail(`missing producer job ${producer.job}`); }
		const header = owner.split("    steps:\n")[0];
		const guard = producer.suite ? "always() && github.event_name != 'push'" : "github.event_name == 'pull_request' || github.event_name == 'merge_group'";
		if (header.match(/^    if: (.+)$/m)?.[1] !== guard) fail("producer job must run in merge_group");
		const steps = owner.split(/\n      - /).slice(1);
		const producing = steps.find((step) => step.startsWith(`name: ${producer.produce}\n`));
		const uploading = steps.filter((step) => step.includes(`          name: ${artifact}\n`));
		if (!producing || !producing.includes(producer.command)) fail("missing producing command");
		if (uploading.length !== 1) fail("requires exactly one producer upload");
		const upload = uploading[0];
		const produceGuard = producer.suite ? `matrix.suite == '${producer.suite}'` : "always()";
		const uploadGuard = producer.suite ? `always() && matrix.suite == '${producer.suite}'` : "always() && steps.lint-inputs.outputs.deadcode != '0'";
		if (producing.match(/^        if: (.+)$/m)?.[1] !== produceGuard) fail("producing step is unreachable in queue");
		if (upload.match(/^        if: (.+)$/m)?.[1] !== uploadGuard) fail("upload is unreachable in queue");
		if (!upload.includes("uses: actions/upload-artifact@v4\n") || !upload.includes(producer.path + "\n")
			|| !/^          retention-days: 14$/m.test(upload)) fail("upload must retain produced file and retention");
		if (producer.suite) {
			if (!job("classify").includes("run_backend: \${{ steps.classify.outputs.run_backend != 'false' }}")
				|| !job("classify").includes("if: github.event_name == 'pull_request'\n        run: go run ./cmd/ciclassify")) fail("queue must select the full coverage matrix");
			const matrix = `include: \${{ fromJSON(needs.classify.outputs.run_backend != 'false' && '[{"suite":"unit","label":"Unit"},{"suite":"functional","label":"Functional"}]' || '[{"suite":"unit","label":"Unit"}]') }}`;
			if (!header.includes(matrix)) fail("required coverage matrix row is missing");
			const output = producer.suite === "unit" ? "GO_UNIT_COVERAGE_PROFILE: .artifacts/unit-coverage/coverage.out" : "FUNCTIONAL_TEST_VIZ_DIR: .artifacts/functional-test-viz";
			if (!producing.includes(output) || producing.includes("continue-on-error:")) fail("coverage command must produce the uploaded file and propagate failure");
		} else {
			if (!producing.includes("LINT_RUN_DEADCODE: \${{ steps.lint-inputs.outputs.deadcode || '1' }}")
				|| !upload.includes("if-no-files-found: error\n")) fail("normalized deadcode production must be required");
			const selector = steps.find((step) => step.includes("        id: lint-inputs\n"));
			if (!selector?.includes("node scripts/ci/backend-lint-workflow.mjs")
				|| !selector.includes('--event "$LINT_EVENT"')
				|| !selector.includes("LINT_EVENT: \${{ github.event_name }}\n")
				|| /^        if:/m.test(selector)) fail("queue selector must execute with the real event");
			for (const paths of [["README.md"], ["ui/src/App.tsx"], ["docs/reference/run.md"], ["pkg/example.go"], [], null, ["unknown"]]) {
				if (selectInputs({ event: "merge_group", baseSha: "a".repeat(40), testedSha: "b".repeat(40), paths }).deadcode !== 1) fail("queue selector omits deadcode");
			}
		}
	}
	const setup = job("classify");
	require(setup, "if: github.event_name == 'pull_request' || github.event_name == 'merge_group'", "PR/queue setup");
	require(setup, "if: github.event_name == 'pull_request'\n        run: go run ./cmd/ciclassify", "PR-only classification");
	if (setup.includes("merge_group.base_sha")) throw new Error("Queue must not classify paths.");
	for (const lane of ["docs_reference", "readme"]) require(setup, `run_${lane}: "true"`, `mandatory ${lane}`);
	for (const lane of ["frontend", "backend", "ui_backend_integration", "api_package", "packaged_factories_package", "model_providers_package"]) {
		require(setup, `run_${lane}: \${{ steps.classify.outputs.run_${lane} != 'false' }}`, `full queue ${lane} fallback`);
	}
	for (const name of ["readme", "frontend", "frontend-browser", "backend-integration", "ui-backend-integration", "backend-coverage", "verification-policy"]) {
		require(job(name), "if: always() && github.event_name != 'push'", `no main ${name} run`);
	}
	require(job("backend-lint"), "if: github.event_name == 'pull_request' || github.event_name == 'merge_group'\n", "PR/queue Backend Lint");
	const coverage = job("backend-coverage");
	require(coverage, "if: always() && github.event_name != 'push'\n", "mandatory Unit Coverage");
	require(coverage, "include: ${{ fromJSON(needs.classify.outputs.run_backend != 'false'", "classified coverage matrix");
	require(coverage, "|| '[{\"suite\":\"unit\",\"label\":\"Unit\"}]') }}", "unit-only PR fallback");
	for (const text of ["models-wire-first.sha256", "models-wire-second.sha256", "diff -u", "Race-check scoped Models components", "Race-check composed scoped Models scenarios", "go test -race", "./tests/functional/models/inference"]) require(coverage, text, "consolidated Models Wire/race guarantees");
	for (const stepName of ["Race-check scoped Models components", "Race-check composed scoped Models scenarios"]) {
		const step = coverage.split(`      - name: ${stepName}\n`)[1]?.split("      - name:")[0];
		require(step ?? "", "if: matrix.suite == 'unit' && needs.classify.outputs.run_backend != 'false'", "selected Models checks in the unit job");
		require(step, "go test -race -v -p 1 -count=1 -timeout 3m", "Models race execution");
		if (step.includes("continue-on-error")) throw new Error("Models proof must propagate failure.");
	}
	// The retained lane explicitly owns Models race proof. Other coverage steps
	// retain main's no-race restriction, including the ACP replacement SDK.
	const withoutModels = coverage.replace(/      - name: Race-check (?:scoped Models components|composed scoped Models scenarios)\n[\s\S]*?(?=      - name:|$)/g, "");
	assert.doesNotMatch(withoutModels, /go test[^\n]* -race/);
	const policy = job("verification-policy");
	require(policy, "BACKEND_MODELS_RESULT: ${{ needs.classify.outputs.run_backend != 'false' && needs.backend-coverage.result || 'skipped' }}", "fail-closed consolidated Models result");
	for (const text of ['RUN_BACKEND_COVERAGE: "true"', "BACKEND_COVERAGE_RESULT: ${{ needs.backend-coverage.result }}", "BACKEND_RESULT: ${{ needs.backend-integration.result }}"]) require(policy, text, "independent mandatory/selected results");
	require(job("development-package"), "(github.event_name == 'merge_group' || (github.event_name == 'pull_request' &&", "queue/full and PR/selected packages");
	require(job("backend-visualizations-publish"), "if: github.event_name == 'push' && github.ref == 'refs/heads/main'", "main-only maintenance");
	require(workflow, "cancel-in-progress: ${{ github.event_name == 'pull_request' }}", "PR-only cancellation");
	const preview = job("backend-visualizations-preview");
	assert.match(preview, /github\.event_name == 'pull_request'/);
	assert.match(preview, /needs\.backend-coverage\.result == 'success'/);
	assert.match(preview, /needs\.verification-policy\.result == 'success'/);
	assert.match(preview, /permissions:\n      contents: read/);
	assert.match(preview, /name: unit-coverage-diagnostics/);
	assert.match(preview, /name: functional-test-diagnostics/);
	assert.match(preview, /BACKEND_VIS_REQUIRE_COVERAGE: "1"/);
	assert.match(preview, /run: make architecture/);
	assert.match(preview, /name: backend-architecture-preview/);
	assert.match(preview, /path: docs\/architecture\/visualizations\//);
	assert.doesNotMatch(preview, /git push|git commit/);
	const publish = job("backend-visualizations-publish");
	assert.match(publish, /github\.event_name == 'push' && github\.ref == 'refs\/heads\/main'/);
	require(publish, "run: node scripts/ci/merge-queue-evidence.mjs", "exact-tree forwarding before publication");
	require(publish, "run: node scripts/ci/main-dependency-cache.mjs", "dependency-only cache maintenance");
	assert.match(publish, /permissions:\n      contents: read/);
	assert.match(publish, /fetch-depth: 2/);
	assert.match(publish, /persist-credentials: false/);
	assert.match(publish, /git diff --quiet HEAD\^ HEAD -- \. ':\(exclude\)docs\/architecture\/visualizations\/\*\*'/);
	assert.match(publish, /BACKEND_VIS_SOURCE_COMMIT: \$\{\{ github\.sha \}\}/);
	assert.match(publish, /BACKEND_VIS_REQUIRE_COVERAGE: "1"/);
	assert.match(publish, /run: make architecture/);
	assert.match(publish, /git add -A docs\/architecture\/visualizations\//);
	assert.match(publish, /git diff --cached --quiet/);
	assert.match(publish, /uses: actions\/create-github-app-token@v2/);
	assert.match(publish, /gh auth setup-git --hostname github\.com/);
	assert.match(publish, /git push origin "HEAD:\$bot_branch"/);
	assert.match(publish, /gh pr create --base main --head "\$bot_branch"/);
	assert.match(publish, /gh pr merge "\$pr_url" --auto\s*$/m);
	assert.doesNotMatch(publish, /--delete-branch/);
	assert.doesNotMatch(publish, /git push origin HEAD:main/);
	assert.match(job("backend-visualizations-publish"), /if: steps\.candidate\.outputs\.changed == 'true'/);
	return { name: "queue-dispatch", status: "pass" };
}
