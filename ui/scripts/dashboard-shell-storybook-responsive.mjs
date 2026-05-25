const OVERFLOW_TOLERANCE_PX = 4;
const STORY_RENDER_TIMEOUT_MS = 30000;

async function expectNoHorizontalOverflow(page, label) {
  const metrics = await page.evaluate(() => ({
    clientWidth: document.documentElement.clientWidth,
    scrollWidth: document.documentElement.scrollWidth,
  }));

  if (metrics.scrollWidth > metrics.clientWidth + OVERFLOW_TOLERANCE_PX) {
    throw new Error(
      `${label} overflowed horizontally: scrollWidth=${metrics.scrollWidth}, clientWidth=${metrics.clientWidth}.`,
    );
  }
}

async function expectVisible(locator, label) {
  if (typeof locator.waitFor === "function") {
    await locator.waitFor({
      state: "visible",
      timeout: STORY_RENDER_TIMEOUT_MS,
    });
    return;
  }

  if (!(await locator.isVisible())) {
    throw new Error(`${label} was not visible.`);
  }
}

async function waitForStoryRegion(page, regionName) {
  const region = page.getByRole("region", { name: regionName });
  await region.waitFor({ state: "visible" });
  return region;
}

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
  const board = await waitForStoryRegion(
    page,
    "you-agent-factory bento board",
  );
  const workTotalsCard = board.getByRole("article", { name: "Work totals" });
  const headerExportButton = toolbar.getByRole("button", {
    name: "Export PNG",
  });
  const timelineSlider = toolbar.getByRole("slider", {
    name: "Timeline tick",
  });
  const timelineStatus = toolbar.getByText(/^\d+\/\d+$/);
  const streamStatus = toolbar.getByRole("status", {
    name: /Event stream (connecting|live)/,
  });
  const moveButton = board.getByRole("button", {
    exact: true,
    name: "Move Work totals",
  });

  await expectVisible(toolbar, "Dashboard summary shell");
  await expectVisible(workTotalsCard, "Work totals grid-card shell");
  await expectVisible(headerExportButton, "Dashboard export button");
  await expectVisible(timelineSlider, "Timeline slider");
  await expectVisible(timelineStatus, "Timeline status");
  await expectVisible(streamStatus, "Dashboard stream status");
  await expectVisible(moveButton, "Work totals move button");
  if (
    await toolbar.getByRole("button", { name: "Return to current tick" }).isVisible()
  ) {
    throw new Error(
      "Dashboard shell header still rendered the retired return-to-current button.",
    );
  }
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
