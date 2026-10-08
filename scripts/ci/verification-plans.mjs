const minutes = (value) => value * 60_000;
const make = (name, target, budget, options = {}) => ({
  name, command: "make", args: [target], timeoutMs: minutes(budget), ...options,
});

export function frontendPlan() {
  return [
    make("component", "ui-component-test", 8, { env: {
      UI_COMPONENT_MAX_DURATION_MS: "300000", UI_COMPONENT_TEST_MAX_WORKERS: "4",
    } }),
    // test:integration builds the dashboard once before the retained runner.
    make("dashboard-browser", "ui-integration-test", 30),
    // The canonical target builds Storybook once and runs the prebuilt checks.
    make("storybook", "ui-storybook-integration-test", 30),
  ];
}

export function workflowPlan() {
  return [
    { name: "lint-behavior", command: "node", args: ["--test", "scripts/ci/workflow-lint.test.mjs", "scripts/ci/verification-sequence.test.mjs"], timeoutMs: minutes(2) },
    { name: "compiler-cache", command: "python3", args: ["scripts/ci/functional-compile-cache.test.py"], timeoutMs: minutes(2) },
    { name: "schema-lint", command: "node", args: ["scripts/ci/workflow-lint.mjs"], timeoutMs: minutes(2) },
  ];
}

export function apiPlan(env = process.env) {
  return [
    make("dependencies", "ui-deps", 10),
    ...["contracts-smoke", "api-smoke", "api-package-verify"].map((target) =>
      make(target, target, 35, { needs: ["dependencies"] })),
    { name: "candidate", command: "node", timeoutMs: minutes(30), args: [
      "scripts/api-package-development-command.mjs", "--action", "dry-run",
      "--event-name", "pull_request", "--output-directory", ".artifacts/api-development-candidate/package",
      "--package-directory", "packages/api", "--pull-request-head-sha", env.PR_HEAD_SHA,
      "--ref", env.PR_SOURCE_REF, "--repository", env.PR_REPOSITORY,
      "--run-id", env.GITHUB_RUN_ID, "--source-commit", env.PR_HEAD_SHA,
      "--workspace-directory", env.GITHUB_WORKSPACE,
    ] },
  ];
}
