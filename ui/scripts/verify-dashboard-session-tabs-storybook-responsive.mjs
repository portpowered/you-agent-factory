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
    page.getByRole("tab", { name: /root \/ default/i }),
    "Default session tab",
  );
  await expectVisible(
    page.getByRole("tab", { name: /root \/ beta/i }),
    "Named session tab",
  );
  await expectVisible(openButton, "Open another session button");
  await expectVisible(
    page.getByText("Active folder: /workspace/root"),
    "Default session folder label",
  );
  await expectNoHorizontalOverflow(
    page,
    `Dashboard session tabs at ${viewport.label}`,
  );
}
