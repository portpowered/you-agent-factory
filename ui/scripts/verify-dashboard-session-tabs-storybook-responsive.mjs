export async function verifyDashboardSessionTabs(
  { expectNoHorizontalOverflow, expectVisible },
  page,
  viewport,
) {
  const tabsNavigation = page.getByRole("navigation", {
    name: "factory sessions",
  });
  const openButton = page.getByRole("button", {
    name: "Open another session",
  });

  await expectVisible(tabsNavigation, "Session tabs navigation");
  await expectVisible(
    page.getByRole("tab", { name: "root / default root" }),
    "Default session tab",
  );
  await expectVisible(
    page.getByRole("tab", { name: "root / beta beta" }),
    "Named session tab",
  );
  await expectVisible(openButton, "Open another session button");
  await page.getByRole("button", { name: /close root \/ default session/i }).click();
  const betaTab = page.getByRole("tab", {
    name: "root / beta beta",
  });
  await expectVisible(betaTab, "Beta session tab after closing the default tab");
  const betaSelected = await betaTab.getAttribute("aria-selected");
  if (betaSelected !== "true") {
    throw new Error("Beta session tab was not selected after closing the default tab.");
  }
  if ((await page.getByRole("tab", { name: "root / default root" }).count()) !== 0) {
    throw new Error("Default session tab remained visible after closing it.");
  }
  await expectNoHorizontalOverflow(
    page,
    `Dashboard session tabs at ${viewport.label}`,
  );
}
