export async function verifyCurrentSelectionPromptHint({
  expectNoHorizontalOverflow,
  expectVisible,
  page,
  viewport,
}) {
  const currentSelection = page.getByRole("article", {
    name: "Current selection",
  });
  await currentSelection.waitFor({ state: "visible" });
  const promptField = currentSelection.getByRole("textbox", { name: "Prompt" });
  const promptEditor = currentSelection
    .locator(".monaco-editor, [data-monaco-editor='workstation-prompt']")
    .first();
  const saveButton = currentSelection.getByRole("button", {
    name: "Save changes",
  });

  await expectVisible(promptEditor, "Monaco prompt editor");
  await expectVisible(
    currentSelection.getByText(
      "Autocomplete is ready with 2 variables for 1 authored input.",
    ),
    "Prompt autocomplete contract summary",
  );
  await expectVisible(
    currentSelection.getByText(
      "Type inside {{ ... }} to see suggestions, or open Monaco completion manually anywhere in the prompt editor.",
    ),
    "Prompt autocomplete inline guidance",
  );

  const removedHelpButtonCount = await currentSelection
    .getByRole("button", { name: /prompt variable help/i })
    .count();
  if (removedHelpButtonCount > 0) {
    throw new Error("Prompt variable help disclosure should be removed.");
  }

  const removedHelpPanelCount = await currentSelection
    .getByText("Available variables")
    .count();
  if (removedHelpPanelCount > 0) {
    throw new Error("Prompt variable help panel content should be removed.");
  }

  const legacySquiggleOverlayCount = await currentSelection
    .locator("mark")
    .count();
  if (legacySquiggleOverlayCount > 0) {
    throw new Error("Legacy prompt squiggle overlay should be removed.");
  }

  await promptField.click({ force: true });
  await page.keyboard.press("ControlOrMeta+A");
  await page.keyboard.type("Use {{ (index .Inputs 1).Payload }}.");
  await expectVisible(
    promptEditor,
    "Monaco prompt editor after invalid prompt entry",
  );
  await expectVisible(
    currentSelection.getByRole("heading", { name: "Prompt diagnostics" }),
    "Prompt diagnostics summary",
  );
  await expectVisible(
    currentSelection.getByText(
      "Save stays disabled until the prompt validates cleanly for this workstation context.",
    ),
    "Prompt validation blocking guidance",
  );
  await expectVisible(
    currentSelection.getByText(".Inputs[1]", { exact: true }),
    "Prompt diagnostics variable path",
  );
  await expectVisible(
    currentSelection.getByText("(index .Inputs 1)", { exact: true }),
    "Prompt diagnostics source text",
  );

  const saveButtonDisabled = await saveButton.isDisabled();
  if (!saveButtonDisabled) {
    throw new Error("Save changes should stay disabled while diagnostics remain.");
  }

  await expectNoHorizontalOverflow(
    page,
    `Current selection prompt hinting at ${viewport.label}`,
  );
}

function expectHeadingBeforePosition(firstRect, secondRect, label) {
  if (firstRect.top > secondRect.top) {
    throw new Error(`${label} rendered out of order.`);
  }
}

async function expectSectionHeaderFrame(expectVisible, heading, label) {
  const headerFrame = heading.locator("xpath=ancestor::div[contains(@class, 'rounded-lg')]").first();
  await expectVisible(headerFrame, `${label} header frame`);
}

export async function verifyCurrentSelectionWorkstationDetailOrder({
  expectNoHorizontalOverflow,
  expectVisible,
  page,
  viewport,
}) {
  const currentSelection = page.getByRole("article", {
    name: "Current selection",
  });
  await currentSelection.waitFor({ state: "visible" });

  const summaryHeading = currentSelection.getByRole("heading", {
    name: "Workstation summary",
  });
  const configurationHeading = currentSelection.getByRole("heading", {
    name: "Configuration",
  });
  const activeWorkHeading = currentSelection.getByRole("heading", {
    name: "Active work",
  });
  const requestHistoryHeading = currentSelection.getByRole("heading", {
    name: "Request history",
  });
  const runHistoryHeading = currentSelection.getByRole("heading", {
    name: "Run history",
  });
  const historyHeading =
    (await requestHistoryHeading.count()) > 0
      ? requestHistoryHeading
      : runHistoryHeading;

  await expectVisible(summaryHeading, "Workstation summary heading");
  await expectVisible(configurationHeading, "Configuration heading");
  await expectVisible(activeWorkHeading, "Active work heading");
  await expectVisible(historyHeading, "History heading");
  await expectSectionHeaderFrame(
    expectVisible,
    summaryHeading,
    "Workstation summary",
  );
  await expectSectionHeaderFrame(
    expectVisible,
    configurationHeading,
    "Configuration",
  );
  await expectSectionHeaderFrame(expectVisible, activeWorkHeading, "Active work");
  await expectSectionHeaderFrame(expectVisible, historyHeading, "History");
  await expectVisible(
    currentSelection.getByText("Input work types"),
    "Workstation summary work-type label",
  );
  await expectVisible(
    currentSelection.getByText("Active runs"),
    "Workstation summary activity-count label",
  );
  await expectVisible(
    currentSelection.getByRole("button", {
      name: "Select work item Active Story",
    }).first(),
    "Active work selection button",
  );

  expectHeadingBeforePosition(
    await summaryHeading.boundingBox(),
    await configurationHeading.boundingBox(),
    "Summary heading before configuration heading",
  );
  expectHeadingBeforePosition(
    await configurationHeading.boundingBox(),
    await activeWorkHeading.boundingBox(),
    "Configuration heading before active work heading",
  );
  expectHeadingBeforePosition(
    await activeWorkHeading.boundingBox(),
    await historyHeading.boundingBox(),
    "Active work heading before history heading",
  );

  await currentSelection.getByRole("button", { name: "Expand" }).click();
  await expectVisible(
    currentSelection.getByRole("button", {
      name: "Select provider session codex / session_id / sess-rejected-story for dispatch dispatch-review-rejected",
    }),
    "History selection button",
  );

  await expectNoHorizontalOverflow(
    page,
    `Current selection workstation detail order at ${viewport.label}`,
  );
}

export async function verifyCurrentSelectionSaveFlow({
  expectNoHorizontalOverflow,
  expectVisible,
  page,
  viewport,
}) {
  const currentSelection = page.getByRole("article", {
    name: "Current selection",
  });
  await currentSelection.waitFor({ state: "visible" });
  const promptField = currentSelection.getByRole("textbox", { name: "Prompt" });
  const saveButton = currentSelection.getByRole("button", {
    name: "Save changes",
  });

  const expandButton = currentSelection.getByRole("button", {
    name: "Expand editable configuration",
  });
  await expandButton.click();
  await expectVisible(promptField, "Workstation prompt field");
  await promptField.click({ force: true });
  await page.keyboard.type(" Browser verified prompt update.");
  await waitForLocatorEnabled(saveButton, "Save changes button");
  await saveButton.click();

  const confirmationDialog = page.getByRole("dialog", {
    name: "Overwrite the running factory definition?",
  });
  await expectVisible(
    confirmationDialog,
    "Editable workstation save confirmation dialog",
  );
  const overwriteButton = confirmationDialog.getByRole("button", {
    name: "Overwrite factory",
  });
  await overwriteButton.click();

  await expectVisible(
    currentSelection.getByText(
      "Running factory saved. The editable workstation values were refreshed to the saved definition.",
    ),
    "Editable workstation save success message",
  );
  const overwriteDialogCount = await page
    .getByRole("dialog", {
      name: "Overwrite the running factory definition?",
    })
    .count();
  if (overwriteDialogCount > 0) {
    throw new Error("Save confirmation dialog should close after a successful save.");
  }

  const saveButtonDisabled = await saveButton.isDisabled();
  if (!saveButtonDisabled) {
    throw new Error("Save changes should disable after the saved draft is refreshed.");
  }

  await expectNoHorizontalOverflow(
    page,
    `Current selection save flow at ${viewport.label}`,
  );
}

async function waitForLocatorEnabled(locator, label) {
  const timeoutAt = Date.now() + 30_000;

  while (Date.now() < timeoutAt) {
    if (!(await locator.isDisabled())) {
      return;
    }

    await new Promise((resolve) => setTimeout(resolve, 50));
  }

  throw new Error(`${label} did not become enabled before timeout.`);
}
