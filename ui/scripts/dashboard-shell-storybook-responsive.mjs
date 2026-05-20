import {
  expectNoHorizontalOverflow,
  expectVisible,
  waitForStoryRegion,
} from "./storybook-responsive-helpers.mjs";

async function dashboardShellStyle(locator) {
  return locator.evaluate((element) => {
    const styles = window.getComputedStyle(element);

    return {
      backgroundColor: styles.backgroundColor,
      borderBottomColor: styles.borderBottomColor,
      borderBottomLeftRadius: styles.borderBottomLeftRadius,
      borderBottomRightRadius: styles.borderBottomRightRadius,
      borderBottomStyle: styles.borderBottomStyle,
      borderBottomWidth: styles.borderBottomWidth,
      borderLeftColor: styles.borderLeftColor,
      borderLeftStyle: styles.borderLeftStyle,
      borderLeftWidth: styles.borderLeftWidth,
      borderRightColor: styles.borderRightColor,
      borderRightStyle: styles.borderRightStyle,
      borderRightWidth: styles.borderRightWidth,
      borderTopColor: styles.borderTopColor,
      borderTopLeftRadius: styles.borderTopLeftRadius,
      borderTopRightRadius: styles.borderTopRightRadius,
      borderTopStyle: styles.borderTopStyle,
      borderTopWidth: styles.borderTopWidth,
      boxShadow: styles.boxShadow,
    };
  });
}

export async function expectMatchingDashboardShellStyles(
  header,
  gridCard,
  label,
) {
  const [headerStyle, gridCardStyle] = await Promise.all([
    dashboardShellStyle(header),
    dashboardShellStyle(gridCard),
  ]);

  for (const [property, headerValue] of Object.entries(headerStyle)) {
    const gridCardValue = gridCardStyle[property];

    if (headerValue !== gridCardValue) {
      throw new Error(
        `${label} ${property} differed: header=${headerValue}, gridCard=${gridCardValue}.`,
      );
    }
  }
}

export async function verifyDashboardShellConsolidation(
  page,
  _dialog,
  viewport,
) {
  const toolbar = await waitForStoryRegion(page, "dashboard summary");
  const board = await waitForStoryRegion(page, "Infinite You bento board");
  const workTotalsCard = board.getByRole("article", { name: "Work totals" });
  const headerExportButton = toolbar.getByRole("button", {
    name: "Export PNG",
  });
  const headerCurrentButton = toolbar.getByRole("button", {
    name: "Return to current tick",
  });
  const streamStatus = toolbar.getByRole("status", {
    name: /Infinite You event stream (connecting|live)/,
  });
  const moveButton = board.getByRole("button", { name: "Move Work totals" });

  await expectVisible(toolbar, "Dashboard summary shell");
  await expectVisible(workTotalsCard, "Work totals grid-card shell");
  await expectVisible(headerExportButton, "Dashboard export button");
  await expectVisible(headerCurrentButton, "Return-to-current button");
  await expectVisible(streamStatus, "Dashboard stream status");
  await expectVisible(moveButton, "Work totals move button");
  await expectMatchingDashboardShellStyles(
    toolbar,
    workTotalsCard,
    `Dashboard shell at ${viewport.label}`,
  );
  await expectNoHorizontalOverflow(
    page,
    `Dashboard shared shell at ${viewport.label}`,
  );
}
