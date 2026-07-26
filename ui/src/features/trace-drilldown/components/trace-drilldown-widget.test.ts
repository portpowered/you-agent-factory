import { expect, test } from "vitest";

import { TraceDrilldownWidget } from "./trace-drilldown-widget";

test("TraceDrilldownWidget keeps the compact trace shell minimum height without forcing full-height scroll", () => {
  const element = TraceDrilldownWidget({
    state: { message: "Select work to inspect trace history.", status: "idle" },
  });

  expect(element.props.className).not.toContain("h-full");
  expect(element.props.className).toContain("min-h-[24rem]");
});
