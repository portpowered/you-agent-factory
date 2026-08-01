import { expect, test } from "vitest";

import {
  isCoverageSourcePath,
  mergeLcovReports,
} from "./merge-lcov-coverage.mjs";

const baseReport = `TN:
SF:src\\lib\\cn.ts
FN:1,cn
FNDA:0,cn
FNF:1
FNH:0
DA:1,0
LF:1
LH:0
end_of_record`;

const bunReport = `TN:
SF:src/lib/cn.ts
FN:1,cn
FNDA:2,cn
FNF:1
FNH:1
DA:1,2
LF:1
LH:1
end_of_record
TN:
SF:src/lib/cn.bun.unit.test.ts
DA:1,1
LF:1
LH:1
end_of_record`;

test("merges Bun source counts into a matching Vitest record once", () => {
  const merged = mergeLcovReports(baseReport, bunReport);

  expect(merged.match(/SF:.*cn\.ts/g)).toHaveLength(1);
  expect(merged).toContain("FNDA:2,cn");
  expect(merged).toContain("DA:1,2");
  expect(merged).toContain("LH:1");
});

test("does not treat a Bun test file as a coverage source", () => {
  const merged = mergeLcovReports("", bunReport);

  expect(merged).not.toContain("cn.bun.unit.test.ts");
  expect(isCoverageSourcePath("src/lib/cn.ts")).toBe(true);
  expect(isCoverageSourcePath("src/lib/cn.bun.unit.test.ts")).toBe(false);
});
