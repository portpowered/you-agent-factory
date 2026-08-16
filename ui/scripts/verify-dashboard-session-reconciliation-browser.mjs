import { chromium } from "playwright";
import { expect } from "playwright/test";

const storybookURL =
  process.env.AGENT_FACTORY_STORYBOOK_URL ?? "http://127.0.0.1:6008";
const storyID =
  "you-agent-factory-dashboard-session-tabs--canonical-list-reconciliation";
const viewports = [
  { height: 844, label: "mobile", width: 390 },
  { height: 900, label: "desktop", width: 1440 },
];

function errorMessage(error) {
  return error instanceof Error && error.message
    ? error.message
    : String(error);
}

async function accessibleTabName(tab) {
  try {
    return await tab.evaluate((element) => {
      const ariaLabel = element.getAttribute("aria-label")?.trim();
      if (ariaLabel) {
        return ariaLabel;
      }

      const textContent = element.textContent?.replace(/\s+/g, " ").trim();
      return textContent || "(unnamed tab)";
    });
  } catch {
    return "(tab name unavailable)";
  }
}

async function selectedTabNames(navigation) {
  try {
    const selectedTabs = navigation.getByRole("tab", { selected: true });
    const selectedTabCount = await selectedTabs.count();
    if (selectedTabCount === 0) {
      return "(none could be identified)";
    }

    const names = [];
    for (let index = 0; index < selectedTabCount; index += 1) {
      names.push(await accessibleTabName(selectedTabs.nth(index)));
    }
    return names.join(", ");
  } catch {
    return "(none could be identified)";
  }
}

async function waitForFocusedTab(tab, action, viewportLabel) {
  try {
    await expect(tab).toBeFocused();
  } catch (error) {
    const expectedTabName = await accessibleTabName(tab);
    throw new Error(
      `${action} key expected the ${expectedTabName} session tab to be focused before the keypress at ${viewportLabel}. Playwright error: ${errorMessage(error)}`,
      { cause: error },
    );
  }
}

async function waitForSelectedTab(tab, navigation, action, viewportLabel) {
  try {
    await expect(tab).toBeVisible();
    await expect(tab).toHaveAttribute("aria-selected", "true");
  } catch (error) {
    const expectedTabName = await accessibleTabName(tab);
    const actualSelectedTabNames = await selectedTabNames(navigation);
    throw new Error(
      `${action} key did not select the expected ${expectedTabName} session tab at ${viewportLabel}. Actually selected tab: ${actualSelectedTabNames}. Playwright error: ${errorMessage(error)}`,
      { cause: error },
    );
  }
}

async function waitForRovingTabState(navigation, action, viewportLabel) {
  try {
    const tabs = navigation.getByRole("tab");
    const tabCount = await tabs.count();
    const selectedTabs = navigation.getByRole("tab", { selected: true });
    const nonSelectedTabs = navigation.getByRole("tab", { selected: false });

    await expect(selectedTabs).toHaveCount(1);
    await expect(nonSelectedTabs).toHaveCount(tabCount - 1);
    await expect(navigation.locator('[role="tab"][tabindex="0"]')).toHaveCount(
      1,
    );
    await expect(navigation.locator('[role="tab"][tabindex="-1"]')).toHaveCount(
      tabCount - 1,
    );
    await expect(selectedTabs.first()).toHaveAttribute("tabindex", "0");

    for (let index = 0; index < tabCount; index += 1) {
      const tab = tabs.nth(index);
      if ((await tab.getAttribute("aria-selected")) === "true") {
        await expect(tab).toHaveAttribute("tabindex", "0");
      } else {
        await expect(tab).toHaveAttribute("aria-selected", "false");
        await expect(tab).toHaveAttribute("tabindex", "-1");
      }
    }
  } catch (error) {
    throw new Error(
      `${action} key found an invalid selected/roving tab state at ${viewportLabel}. Playwright error: ${errorMessage(error)}`,
      { cause: error },
    );
  }
}

async function verifyViewport(browser, viewport) {
  const context = await browser.newContext({ viewport });
  const page = await context.newPage();
  page.setDefaultTimeout(30_000);

  try {
    await page.goto(
      new URL(
        `/iframe.html?id=${storyID}&viewMode=story`,
        storybookURL,
      ).toString(),
      { timeout: 30_000, waitUntil: "domcontentloaded" },
    );

    const navigation = page.getByRole("navigation", {
      name: "factory sessions",
    });
    await navigation.waitFor({ state: "visible" });

    if ((await navigation.getByRole("tab").count()) !== 2) {
      throw new Error(`Expected two canonical tabs at ${viewport.label}.`);
    }
    if ((await page.getByText("~default", { exact: true }).count()) !== 0) {
      throw new Error(
        `The default selector rendered as a tab at ${viewport.label}.`,
      );
    }

    await page.getByRole("button", { name: "Refresh sessions" }).click();
    await navigation.getByRole("tab", { name: "created" }).waitFor({
      state: "visible",
    });
    if (
      (await navigation.getByRole("tab", { name: "secondary" }).count()) !== 0
    ) {
      throw new Error(
        `The stale session remained after refresh at ${viewport.label}.`,
      );
    }
    if (
      (await navigation.getByRole("tab", { name: "removed" }).count()) !== 0
    ) {
      throw new Error(
        `The removed session rendered after refresh at ${viewport.label}.`,
      );
    }

    const refreshButton = page.getByRole("button", {
      name: "Refresh sessions",
    });
    const failRefreshButton = page.getByRole("button", {
      name: "Fail refresh",
    });
    const tabs = navigation.getByRole("tab");
    const firstTab = tabs.first();
    const lastTab = tabs.last();
    await waitForSelectedTab(firstTab, navigation, "Initial", viewport.label);
    await waitForRovingTabState(navigation, "Initial", viewport.label);
    await expect(refreshButton).toBeFocused();
    await page.keyboard.press("Tab");
    await expect(failRefreshButton).toBeFocused();
    await page.keyboard.press("Tab");
    await waitForFocusedTab(firstTab, "End", viewport.label);
    await page.keyboard.press("End");
    await waitForSelectedTab(lastTab, navigation, "End", viewport.label);
    await waitForRovingTabState(navigation, "End", viewport.label);
    await waitForFocusedTab(lastTab, "Home", viewport.label);
    await page.keyboard.press("Home");
    await waitForSelectedTab(firstTab, navigation, "Home", viewport.label);
    await waitForRovingTabState(navigation, "Home", viewport.label);

    await page.getByRole("button", { name: "Fail refresh" }).click();
    await page
      .getByText("Factory sessions unavailable", { exact: true })
      .waitFor({
        state: "visible",
      });
    if (
      (await navigation.getByRole("tab", { name: "created" }).count()) !== 1
    ) {
      throw new Error(
        `Retained tabs disappeared after refresh failure at ${viewport.label}.`,
      );
    }
    await page.getByRole("button", { name: "Retry sessions" }).click();
    await page
      .getByText("Factory sessions unavailable", { exact: true })
      .waitFor({
        state: "hidden",
      });

    const metrics = await navigation.evaluate((element) => ({
      pageOverflows: document.documentElement.scrollWidth > window.innerWidth,
      tablistOverflows: element.scrollWidth > element.clientWidth,
    }));
    if (
      metrics.pageOverflows ||
      (viewport.width > 600 && metrics.tablistOverflows)
    ) {
      throw new Error(`Session list layout overflowed at ${viewport.label}.`);
    }

    console.log(
      `Verified canonical session reconciliation at ${viewport.label}.`,
    );
  } finally {
    await context.close();
  }
}

const browser = await chromium.launch({ headless: true });
try {
  for (const viewport of viewports) {
    await verifyViewport(browser, viewport);
  }
  console.log("Dashboard session reconciliation browser verification passed.");
} finally {
  await browser.close();
}
