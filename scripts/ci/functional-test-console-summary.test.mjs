import assert from "node:assert/strict";
import test from "node:test";

import { renderFunctionalTestConsoleSummary } from "./functional-test-console-summary.mjs";

test("renders only pkg coverage and functional top-level test latencies", () => {
	const output = renderFunctionalTestConsoleSummary(
		{
			packages: [
				{
					package: "github.com/portpowered/infinite-you/pkg/zeta",
					coveragePercent: 50,
				},
				{
					package: "github.com/portpowered/infinite-you/cmd/factory",
					coveragePercent: 90,
				},
				{
					package: "github.com/portpowered/infinite-you/pkg/alpha",
					coveragePercent: 75.25,
				},
			],
		},
		{
			tests: [
				{
					package:
						"github.com/portpowered/infinite-you/tests/functional/work/beta",
					test: "TestSlow",
					seconds: 2.5,
					outcome: "fail",
				},
				{
					package: "github.com/portpowered/infinite-you/pkg/alpha",
					test: "TestUnit",
					seconds: 0.1,
					outcome: "pass",
				},
				{
					package:
						"github.com/portpowered/infinite-you/tests/functional/work/alpha",
					test: "TestQuick",
					seconds: 0.1254,
					outcome: "pass",
				},
			],
		},
	);

	assert.equal(
		output,
		[
			"Functional coverage for pkg/:",
			"pkg/alpha 75.3%",
			"pkg/zeta 50.0%",
			"",
			"Functional test latencies:",
			"tests/functional/work/alpha TestQuick 0.125s",
			"tests/functional/work/beta TestSlow 2.500s",
			"",
		].join("\n"),
	);
	assert.doesNotMatch(output, /pass|fail|cmd\/factory|TestUnit/);
});

test("returns no output when neither report is available", () => {
	assert.equal(renderFunctionalTestConsoleSummary(null, null), "");
});
