import assert from "node:assert/strict";

// Workflow composition is checked in the static lane, separately from the
// component tests of policy, artifact forwarding, and dependency warming.
export function validateQueueDispatch(workflow) {
	const job = (name) => {
		const match = workflow.match(new RegExp(`\\n  ${name}:\\n([\\s\\S]*?)(?=\\n  [a-z0-9_-]+:\\n|$)`));
		if (!match) throw new Error(`Dispatch is missing ${name}.`);
		return match[1];
	};
	const require = (text, expected, label) => {
		if (!text.includes(expected)) throw new Error(`Dispatch must retain ${label}.`);
	};
	const setup = job("classify");
	require(setup, "if: github.event_name == 'pull_request' || github.event_name == 'merge_group'", "PR/queue setup");
	require(setup, "if: github.event_name == 'pull_request'\n        run: go run ./cmd/ciclassify", "PR-only classification");
	if (setup.includes("merge_group.base_sha")) throw new Error("Queue must not classify paths.");
	for (const lane of ["docs_reference", "readme"]) require(setup, `run_${lane}: "true"`, `mandatory ${lane}`);
	for (const lane of ["frontend", "backend", "backend_conformance", "ui_backend_integration", "api_package", "packaged_factories_package", "model_providers_package"]) {
		require(setup, `run_${lane}: \${{ steps.classify.outputs.run_${lane} != 'false' }}`, `full queue ${lane} fallback`);
	}
	for (const name of ["readme", "frontend", "frontend-browser", "backend-models-verification", "backend-conformance", "backend-integration", "tts-clean-install-windows", "ui-backend-integration", "backend-coverage", "verification-policy"]) {
		require(job(name), "if: always() && github.event_name != 'push'", `no main ${name} run`);
	}
	require(job("backend-lint"), "if: github.event_name == 'pull_request' || github.event_name == 'merge_group'\n", "PR/queue Backend Lint");
	const coverage = job("backend-coverage");
	require(coverage, "if: always() && github.event_name != 'push'\n", "mandatory Unit Coverage");
	require(coverage, "include: ${{ fromJSON(needs.classify.outputs.run_backend != 'false'", "classified coverage matrix");
	require(coverage, "|| '[{\"suite\":\"unit\",\"label\":\"Unit\"}]') }}", "unit-only PR fallback");
	const policy = job("verification-policy");
	for (const text of ['RUN_BACKEND_COVERAGE: "true"', "BACKEND_COVERAGE_RESULT: ${{ needs.backend-coverage.result }}", "BACKEND_RESULT: ${{ needs.backend-integration.result }}", "BACKEND_TTS_RESULT: ${{ needs.tts-clean-install-windows.result }}"]) require(policy, text, "independent mandatory/selected results");
	require(job("development-package"), "(github.event_name == 'merge_group' || (github.event_name == 'pull_request' &&", "queue/full and PR/selected packages");
	require(job("backend-visualizations-publish"), "needs: main-evidence", "forwarded architecture evidence");
	for (const name of ["main-evidence", "main-dependency-cache"]) require(job(name), "if: github.event_name == 'push' && github.ref == 'refs/heads/main'", "main-only maintenance");
	require(workflow, "cancel-in-progress: ${{ github.event_name == 'pull_request' }}", "PR-only cancellation");
	const conformanceJob = job("backend-conformance");
	const policyJob = job("verification-policy");
	const offlineStep = conformanceJob.indexOf("run: make test-backend-conformance");
	const liveStep = conformanceJob.indexOf("run: make test-backend-conformance-live");

	assert.match(
		conformanceJob,
		/if: always\(\) && github\.event_name != 'push' && needs\.classify\.outputs\.run_backend_conformance != 'false'/,
	);
	assert.match(conformanceJob, /timeout-minutes: 10/);
	assert.ok(offlineStep >= 0, "offline backend conformance step is missing");
	assert.ok(liveStep > offlineStep, "live validation must follow offline conformance");
	assert.doesNotMatch(conformanceJob, /continue-on-error:\s*true/);

	assert.match(policyJob, /needs: \[[^\]]*\bbackend-conformance\b[^\]]*\]/s);
	assert.match(
		policyJob,
		/BACKEND_CONFORMANCE_RESULT: \$\{\{ needs\.backend-conformance\.result \}\}/,
	);
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
	assert.match(publish, /needs\.main-evidence\.result == 'success'/);
	assert.match(publish, /needs: main-evidence/);
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
