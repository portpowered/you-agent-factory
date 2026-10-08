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
	for (const name of ["readme", "frontend", "backend-models-verification", "backend-conformance", "backend-integration", "tts-clean-install-windows", "ui-backend-integration", "backend-coverage", "verification-policy"]) {
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
	return { name: "queue-dispatch", status: "pass" };
}
