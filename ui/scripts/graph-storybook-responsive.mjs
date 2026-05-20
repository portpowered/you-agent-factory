import process from "node:process";

const STORYBOOK_HOST = process.env.AGENT_FACTORY_STORYBOOK_HOST ?? "127.0.0.1";
const STORYBOOK_PORT = process.env.AGENT_FACTORY_STORYBOOK_PORT ?? "6008";
const STORYBOOK_URL = `http://${STORYBOOK_HOST}:${STORYBOOK_PORT}`;

const DASHBOARD_GRAPH_CONTROL_STYLE_KEYS = [
  "backgroundColor",
  "borderBottomColor",
  "borderBottomStyle",
  "borderBottomWidth",
  "borderLeftColor",
  "borderLeftStyle",
  "borderLeftWidth",
  "borderRightColor",
  "borderRightStyle",
  "borderRightWidth",
  "borderTopColor",
  "borderTopStyle",
  "borderTopWidth",
  "borderTopLeftRadius",
  "borderTopRightRadius",
  "boxShadow",
];

function storyUrl(storyId) {
  return `${STORYBOOK_URL}/iframe.html?id=${storyId}&viewMode=story`;
}

async function readComputedStyle(locator, keys) {
  return locator.evaluate(
    (element, selectedKeys) => {
      const computedStyle = window.getComputedStyle(element);

      return Object.fromEntries(
        selectedKeys.map((key) => [key, computedStyle[key]]),
      );
    },
    keys,
  );
}

export async function expectMatchingDashboardGraphStyles(
  expectedStyles,
  actualStyles,
  label,
) {
  for (const [styleKey, expectedValue] of Object.entries(expectedStyles)) {
    if (actualStyles[styleKey] !== expectedValue) {
      throw new Error(
        `${label} ${styleKey} differed: expected=${expectedValue}, actual=${actualStyles[styleKey]}.`,
      );
    }
  }
}

async function readDashboardGraphControlsStyle(region, label, expectVisible) {
  const controls = region.locator(".react-flow__controls").first();

  await expectVisible(controls, `${label} graph controls`);

  return readComputedStyle(controls, DASHBOARD_GRAPH_CONTROL_STYLE_KEYS);
}

export async function verifyTraceFactoryGraphVisualConsistency({
  expectNoHorizontalOverflow,
  expectVisible,
  page,
  viewport,
  waitForStoryRender,
}) {
  const factoryViewport = page.getByRole("region", {
    name: "Work graph viewport",
  });

  await factoryViewport.waitFor({ state: "visible" });
  await expectVisible(
    page.getByText("Observe mode"),
    "Factory graph observe-mode badge",
  );
  const factoryControlsStyle = await readDashboardGraphControlsStyle(
    factoryViewport,
    "Factory",
    expectVisible,
  );
  await expectNoHorizontalOverflow(page, `Factory graph at ${viewport.label}`);

  await page.goto(
    storyUrl("agent-factory-dashboard-trace-graph-surfaces--visual-consistency"),
    { waitUntil: "domcontentloaded" },
  );
  await waitForStoryRender(page);

  const dispatchViewport = page.getByRole("region", {
    name: "Dispatch relationship graph",
  });
  const relationViewport = page.getByRole("region", {
    name: "Batch relation graph",
  });

  await expectVisible(dispatchViewport, "Trace dispatch graph region");
  await expectVisible(relationViewport, "Trace relation graph region");
  await expectVisible(
    dispatchViewport.getByText('Out: story:"Reviewed Story"'),
    "Trace dispatch story content",
  );
  await expectVisible(
    relationViewport.getByText("Repair story"),
    "Trace relation story content",
  );

  const dispatchControlsStyle = await readDashboardGraphControlsStyle(
    dispatchViewport,
    "Trace dispatch",
    expectVisible,
  );
  const relationControlsStyle = await readDashboardGraphControlsStyle(
    relationViewport,
    "Trace relation",
    expectVisible,
  );

  await expectMatchingDashboardGraphStyles(
    factoryControlsStyle,
    dispatchControlsStyle,
    "Trace dispatch graph controls",
  );
  await expectMatchingDashboardGraphStyles(
    factoryControlsStyle,
    relationControlsStyle,
    "Trace relation graph controls",
  );

  await expectNoHorizontalOverflow(
    page,
    `Trace graph consolidation at ${viewport.label}`,
  );
}
