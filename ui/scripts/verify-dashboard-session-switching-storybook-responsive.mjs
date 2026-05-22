export async function verifyDashboardSessionSwitching(
  { expectNoHorizontalOverflow, expectVisible },
  page,
  viewport,
) {
  const betaTab = page.getByRole("tab", {
    name: "root / beta beta",
  });

  await betaTab.click();
  await expectVisible(
    page.getByRole("button", { name: /Beta Story/ }),
    "Beta session active work item",
  );
  const betaSelected = await betaTab.getAttribute("aria-selected");
  if (betaSelected !== "true") {
    throw new Error("Beta session tab was not selected after switching.");
  }

  if ((await page.getByRole("button", { name: /Active Story/ }).count()) !== 0) {
    throw new Error("Default session active work remained visible after switching tabs.");
  }

  await expectNoHorizontalOverflow(
    page,
    `Dashboard session switching at ${viewport.label}`,
  );
}
