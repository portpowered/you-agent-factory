import { TraceDrilldownWidget } from "./trace-drilldown-widget";

test("TraceDrilldownWidget keeps the compact trace shell minimum height inline on the bento card", () => {
  const element = TraceDrilldownWidget({
    state: { message: "Select work to inspect trace history.", status: "idle" },
  });

  expect(element.props.className).toContain("h-full");
  expect(element.props.className).toContain("min-h-[24rem]");
});
