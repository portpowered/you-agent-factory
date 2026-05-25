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
    page.getByRole("tab", { name: "root" }),
    "Default session tab",
  );
  await expectVisible(
    page.getByRole("tab", { name: "beta" }),
    "Named beta session tab",
  );
  await expectVisible(
    page.getByRole("button", { name: "Close beta session" }),
    "Inactive beta close button",
  );
  await expectVisible(
    page.getByRole("button", { name: "Close root session" }),
    "Active session close button",
  );
  await expectVisible(openButton, "Open another session button");
  await expectNoHorizontalOverflow(
    page,
    `Dashboard session tabs at ${viewport.label}`,
  );
}
