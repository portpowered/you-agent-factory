export async function verifyObserverGraphParity(
  { expectNoHorizontalOverflow, expectVisible },
  page,
  viewport,
) {
  const currentActivityRegion = page.getByRole("region", {
    name: "Current activity",
  });
  const reviewWorkstation = page.getByRole("button", {
    name: "Select Review workstation",
  });
  const activeWorkItem = page.getByRole("button", { name: /Active Story/ });

  await expectVisible(currentActivityRegion, "Current activity region");
  await expectVisible(reviewWorkstation, "Observer review workstation");
  await expectVisible(activeWorkItem, "Observer active work item");
  await expectVisible(
    reviewWorkstation.getByRole("img", { name: "Repeater workstation" }),
    "Observer workstation semantic icon",
  );
  await expectNoHorizontalOverflow(
    page,
    `Observer graph parity at ${viewport.label}`,
  );
}

export async function verifyEditorGraphParity(
  { expectNoHorizontalOverflow, expectVisible },
  page,
  viewport,
) {
  const allPreset = page.getByRole("button", { name: "All" });
  const workflowPreset = page.getByRole("button", { name: "Workflow" });
  const infrastructurePreset = page.getByRole("button", {
    name: "Infrastructure",
  });

  await expectVisible(allPreset, "All visibility preset");
  await expectVisible(workflowPreset, "Workflow visibility preset");
  await expectVisible(
    infrastructurePreset,
    "Infrastructure visibility preset",
  );
  await expectVisible(page.getByTitle("review"), "Editor workstation node");

  await infrastructurePreset.click();
  await expectVisible(page.getByTitle("gpu"), "Infrastructure resource node");

  await workflowPreset.click();
  await page.getByTitle("review").waitFor({ state: "visible" });
  if (await page.getByTitle("gpu").isVisible()) {
    throw new Error(
      "Workflow preset should hide infrastructure-only resource nodes in editor parity verification.",
    );
  }

  await expectNoHorizontalOverflow(
    page,
    `Editor graph parity at ${viewport.label}`,
  );
}
