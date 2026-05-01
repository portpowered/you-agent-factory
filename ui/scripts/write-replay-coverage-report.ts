import {
  buildReplayCoverageReport,
  formatReplayCoverageReportMarkdown,
  validateReplayCoverageReport,
} from "../src/testing/replay-fixture-catalog";

const report = buildReplayCoverageReport();
const validationIssues = validateReplayCoverageReport(report);
const expectedContent = formatReplayCoverageReportMarkdown(report);
const checkOnly = process.argv.includes("--check");

if (validationIssues.length > 0) {
  console.error("Replay coverage metadata is invalid:");
  for (const issue of validationIssues) {
    console.error(`- ${issue}`);
  }
  process.exit(1);
}

if (checkOnly) {
  process.exit(0);
}

process.stdout.write(expectedContent);
