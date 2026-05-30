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
  for (const workstationName of ["Plan", "Implement", "Review"]) {
    const workstationButton = page.getByRole("button", {
      name: `Select ${workstationName} workstation`,
    });
    await workstationButton.focus();
    await page.keyboard.press("Enter");
  }
  const promptField = currentSelection.getByRole("textbox", { name: "Prompt" });
  const saveButton = currentSelection.getByRole("button", {
    name: "Save changes",
  });
  const expandEditableConfigurationButton = currentSelection.getByRole(
    "button",
    {
      name: "Expand editable configuration",
    },
  );
  await expandEditableConfigurationButton.focus();
  await page.keyboard.press("Enter");

  await expectVisible(promptField, "Prompt field");
  await expectVisible(
    currentSelection.getByText(
      "Autocomplete is ready with 15 variables for 1 authored input.",
    ),
    "Prompt autocomplete contract summary",
  );
  if (viewport.label === "desktop") {
    await verifyPromptEditorAutocompleteCompletion({
      expectVisible,
      page,
    });
  }

  const expandPromptVariableHelpButton = currentSelection.getByRole("button", {
    name: "Open prompt variable help",
  });
  await expandPromptVariableHelpButton.focus();
  await page.keyboard.press("Enter");
  await expectVisible(
    currentSelection.getByText(
      "Suggestions appear only while typing inside {{ ... }}.",
    ),
    "Prompt autocomplete inline guidance",
  );

  await expectVisible(
    currentSelection.getByRole("heading", { name: "Available variables" }),
    "Prompt available variables heading",
  );
  await expectVisible(
    currentSelection.getByText(".WorkID", { exact: true }),
    "Prompt available variable path",
  );
  await expectVisible(
    currentSelection.getByRole("heading", { name: "Unavailable access" }),
    "Prompt unavailable access heading",
  );

  const legacySquiggleOverlayCount = await currentSelection
    .locator("mark")
    .count();
  if (legacySquiggleOverlayCount > 0) {
    throw new Error("Legacy prompt squiggle overlay should be removed.");
  }

  await focusPromptEditor(page);
  await page.keyboard.press("ControlOrMeta+A");
  await page.keyboard.type("Use {{ (index .Inputs 1).Payload }}.");
  await expectVisible(promptField, "Prompt field after invalid prompt entry");
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
    throw new Error(
      "Save changes should stay disabled while diagnostics remain.",
    );
  }

  await expectNoHorizontalOverflow(
    page,
    `Current selection prompt hinting at ${viewport.label}`,
  );
}

async function verifyPromptEditorAutocompleteCompletion({
  expectVisible,
  page,
}) {
  await focusPromptEditor(page);
  await page.keyboard.press("ControlOrMeta+A");
  await page.keyboard.type("Use {{ (index .Inputs 0).");
  await page.keyboard.press("Control+Space");

  const suggestionWidget = page.locator(".suggest-widget.visible");
  await expectVisible(
    suggestionWidget.locator(".monaco-list-row").filter({ hasText: "Payload" }),
    "Prompt autocomplete expanded input-field suggestion",
  );

  const nameSuggestion = suggestionWidget
    .locator(".monaco-list-row")
    .filter({ hasText: "Name" });
  await expectVisible(
    nameSuggestion,
    "Prompt autocomplete prefix-based input-field suggestion",
  );
  await nameSuggestion.click();
  await expectVisible(
    page
      .locator('[data-monaco-editor="workstation-prompt"] .view-line')
      .filter({ hasText: "(index .Inputs 0).Name" }),
    "Prompt editor accepted prefix-based input-field completion",
  );
}

async function focusPromptEditor(page) {
  await page
    .locator('[data-monaco-editor="workstation-prompt"] .native-edit-context')
    .evaluate((element) => {
      element.focus();
    });
}

function expectHeadingBeforePosition(firstRect, secondRect, label) {
  if (firstRect.top > secondRect.top) {
    throw new Error(`${label} rendered out of order.`);
  }
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
  if (!(await currentSelection.isVisible().catch(() => false))) {
    const selectReviewWorkstationButton = page.getByRole("button", {
      name: "Select Review workstation",
    });
    if ((await selectReviewWorkstationButton.count()) > 0) {
      await selectReviewWorkstationButton.click();
    }
  }
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
  await expectVisible(
    currentSelection.getByText("Input work types"),
    "Workstation summary work-type label",
  );
  await expectVisible(
    currentSelection.getByText("Active runs"),
    "Workstation summary activity-count label",
  );
  await expectVisible(
    currentSelection
      .getByRole("button", {
        name: "Select work item Active Story",
      })
      .first(),
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

  const historySection = historyHeading.locator("xpath=ancestor::section[1]");
  const historyExpandButton = historySection.getByRole("button", {
    name: "Expand",
  });
  await historyExpandButton.focus();
  await page.keyboard.press("Enter");
  await expectVisible(
    historySection.getByRole("button", {
      name: "Select provider session codex / Session ID / sess-rejected-story for dispatch dispatch-review-rejected",
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
  const workerField = currentSelection.getByRole("combobox", {
    name: "Worker",
  });
  const saveButton = currentSelection.getByRole("button", {
    name: "Save changes",
  });

  const expandButton = currentSelection.getByRole("button", {
    name: "Expand editable configuration",
  });
  await expandButton.click();
  await expectVisible(promptField, "Workstation prompt field");
  await workerField.selectOption("planner");
  await promptField.click({ force: true });
  await page.keyboard.press("ControlOrMeta+A");
  await page.keyboard.insertText("Browser verified prompt update.");
  await waitForEditableConfigurationReadyToSave(currentSelection, saveButton);
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
    throw new Error(
      "Save confirmation dialog should close after a successful save.",
    );
  }

  const saveButtonDisabled = await saveButton.isDisabled();
  if (!saveButtonDisabled) {
    throw new Error(
      "Save changes should disable after the saved draft is refreshed.",
    );
  }

  await expectNoHorizontalOverflow(
    page,
    `Current selection save flow at ${viewport.label}`,
  );
}

async function waitForEditableConfigurationReadyToSave(
  currentSelection,
  saveButton,
) {
  const timeoutAt = Date.now() + 30_000;
  const validationMessage = currentSelection.getByText(
    "Validating prompt variables for the current draft.",
  );

  while (Date.now() < timeoutAt) {
    const saveButtonDisabled = await saveButton.isDisabled();
    const validatingCount = await validationMessage.count();
    if (!saveButtonDisabled && validatingCount === 0) {
      return;
    }

    await new Promise((resolve) => setTimeout(resolve, 50));
  }

  throw new Error(
    "Editable configuration did not finish validation with an enabled save button before timeout.",
  );
}
