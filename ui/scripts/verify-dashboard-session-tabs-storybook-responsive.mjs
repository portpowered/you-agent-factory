export async function verifyDashboardSessionTabs(
  { expectNoHorizontalOverflow, expectVisible, waitForDialog },
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

  await openButton.click();
  const dialog = await waitForDialog(page, "Open factory session");
  const folderField = dialog.getByRole("textbox", { name: "Factory folder" });
  await folderField.fill("/workspace/catalog");
  await dialog.getByRole("button", { name: "Inspect folder" }).click();

  const targetPicker = page.getByRole("region", {
    name: "Pick a runnable target",
  });
  await expectVisible(targetPicker, "Session target picker");
  await expectVisible(
    targetPicker.getByText("Choose one runnable target from this folder."),
    "Session target picker hint",
  );
  await targetPicker
    .getByRole("button", { name: "Catalog / review catalog" })
    .click();

  const reviewTab = page.getByRole("tab", {
    name: "catalog / review catalog",
  });
  await expectVisible(reviewTab, "Opened review session tab");
  const selected = await reviewTab.getAttribute("aria-selected");
  if (selected !== "true") {
    throw new Error("Opened review session tab was not selected.");
  }
  await expectVisible(
    page.getByText("Active folder: /workspace/catalog"),
    "Opened review session path",
  );
  await expectNoHorizontalOverflow(
    page,
    `Dashboard session tabs at ${viewport.label}`,
  );
}
