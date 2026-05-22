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
