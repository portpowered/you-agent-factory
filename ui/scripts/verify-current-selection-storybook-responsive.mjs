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
  const helpButton = currentSelection.getByRole("button", {
    name: "Close prompt variable help",
  });
  const promptField = currentSelection.getByRole("textbox", { name: "Prompt" });
  const saveButton = currentSelection.getByRole("button", {
    name: "Save changes",
  });

  await expectVisible(helpButton, "Prompt variable help toggle");
  await expectVisible(
    currentSelection.getByText("This workstation exposes 1 authored input."),
    "Prompt help input-count summary",
  );
  await expectVisible(
    currentSelection.getByText("Available variables"),
    "Prompt help available variables heading",
  );
  await expectVisible(
    currentSelection.getByText("{{ .WorkID }}"),
    "Prompt help example snippet",
  );

  await promptField.focus();
  await promptField.fill("Use {{ (index .Inputs 1).Payload }}.");
  await expectVisible(
    currentSelection.getByRole("heading", { name: "Prompt diagnostics" }),
    "Prompt diagnostics summary",
  );
  await expectVisible(
    currentSelection.getByText(".Inputs[1]", { exact: true }),
    "Prompt diagnostics variable path",
  );
  await expectVisible(
    currentSelection.locator("mark").filter({ hasText: "(index .Inputs 1)" }),
    "Prompt squiggle overlay",
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
