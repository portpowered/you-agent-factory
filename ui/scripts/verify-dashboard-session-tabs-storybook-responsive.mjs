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
  const folderField = dialog.getByRole("textbox", {
    name: "Factory folder",
  });
  await folderField.fill("/workspace/catalog");
  await dialog.getByRole("button", { name: "Inspect folder" }).click();

  const targetPicker = page.getByRole("region", {
    name: "Pick a runnable target",
  });
  await expectVisible(targetPicker, "Target picker");
  await expectVisible(
    targetPicker.getByText("Choose one runnable target from this folder."),
    "Target picker helper text",
  );

  await targetPicker
    .getByRole("button", { name: "Catalog / review catalog" })
    .click();

  const reviewTab = page.getByRole("tab", {
    name: "catalog / review catalog",
  });
  await expectVisible(reviewTab, "Review session tab");
  const reviewSelected = await reviewTab.getAttribute("aria-selected");
  if (reviewSelected !== "true") {
    throw new Error("Review session tab was not selected after opening it.");
  }
  await expectVisible(
    page.getByText("Active folder: /workspace/catalog"),
    "Opened session folder label",
  );

  await page
    .getByRole("button", { name: /close catalog \/ review session/i })
    .click();

  const defaultTab = page.getByRole("tab", {
    name: "root / default root",
  });
  await expectVisible(defaultTab, "Default session tab after closing review session");
  const defaultSelected = await defaultTab.getAttribute("aria-selected");
  if (defaultSelected !== "true") {
    throw new Error("Default session tab was not restored after closing the review tab.");
  }
  await expectVisible(
    page.getByText("Active folder: /workspace/root"),
    "Default session folder label",
  );
  await expectNoHorizontalOverflow(
    page,
    `Dashboard session tabs at ${viewport.label}`,
  );
}
