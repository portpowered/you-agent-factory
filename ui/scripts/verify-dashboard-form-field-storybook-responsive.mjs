export const DASHBOARD_FORM_FIELD_RESPONSIVE_VIEWPORTS = [
  { height: 844, label: "mobile", width: 390 },
  { height: 900, label: "desktop", width: 1440 },
];

export const DASHBOARD_FORM_FIELD_STORY_CHECKS = [
  {
    id: "you-agent-factory-dashboard-export-factory-dialog--validation",
    label: "export factory dialog validation",
    verify: verifyExportFactoryDialogValidation,
  },
  {
    id: "agent-factory-dashboard-submit-work-card--stable-action-alignment",
    label: "submit work card validation alignment",
    verify: verifySubmitWorkCardValidationAlignment,
  },
];

export async function verifyExportFactoryDialogValidation({
  expectNoHorizontalOverflow: expectNoOverflow,
  expectVisible: expectStoryVisible,
  page,
  viewport,
}) {
  const dialog = page.getByRole("dialog", { name: "Export factory" });
  await expectStoryVisible(dialog, "Export factory dialog");

  await expectStoryVisible(
    dialog.getByRole("textbox", { name: "Factory name" }),
    "Export factory name input",
  );
  await expectStoryVisible(
    dialog.getByLabel("Cover image"),
    "Export factory cover image input",
  );
  await expectStoryVisible(
    dialog.getByText("Enter a factory name before exporting."),
    "Export factory name validation",
  );
  await expectStoryVisible(
    dialog.getByText("Choose a cover image before exporting."),
    "Export factory image validation",
  );

  const nameInput = dialog.getByRole("textbox", { name: "Factory name" });
  const nameError = dialog.getByText("Enter a factory name before exporting.");
  const imageInput = dialog.getByLabel("Cover image");
  const imageError = dialog.getByText("Choose a cover image before exporting.");

  await nameInput.evaluate(
    (element, errorId) => {
      if (element.getAttribute("aria-invalid") !== "true") {
        throw new Error(
          "expected export factory name input to be aria-invalid",
        );
      }
      if (element.getAttribute("aria-describedby") !== errorId) {
        throw new Error(
          "expected export factory name input aria-describedby to reference the validation message",
        );
      }
    },
    await nameError.getAttribute("id"),
  );
  await imageInput.evaluate(
    (element, errorId) => {
      if (element.getAttribute("aria-invalid") !== "true") {
        throw new Error("expected export cover image input to be aria-invalid");
      }
      if (element.getAttribute("aria-describedby") !== errorId) {
        throw new Error(
          "expected export cover image input aria-describedby to reference the validation message",
        );
      }
    },
    await imageError.getAttribute("id"),
  );

  await expectNoOverflow(
    page,
    `Export factory dialog validation at ${viewport.label}`,
  );
}

export async function verifySubmitWorkCardValidationAlignment({
  expectNoHorizontalOverflow: expectNoOverflow,
  expectVisible: expectStoryVisible,
  page,
  viewport,
}) {
  const cards = page.getByRole("article", { name: "Submit work" });
  const validationCard = cards.last();
  await expectStoryVisible(validationCard, "Submit work validation card");

  await expectStoryVisible(
    validationCard.getByText("Enter a request name before submitting."),
    "Submit work request name validation",
  );
  await expectStoryVisible(
    validationCard.getByText("Choose a work type before submitting."),
    "Submit work type validation",
  );

  const requestName = validationCard.getByRole("textbox", {
    name: "Request name (required)",
  });
  const requestError = validationCard.getByText(
    "Enter a request name before submitting.",
  );
  const workType = validationCard.getByRole("combobox", {
    name: /Work type/,
  });
  const workTypeError = validationCard.getByText(
    "Choose a work type before submitting.",
  );

  await requestName.evaluate(
    (element, errorId) => {
      if (element.getAttribute("aria-invalid") !== "true") {
        throw new Error(
          "expected submit work request name input to be aria-invalid",
        );
      }
      if (element.getAttribute("aria-describedby") !== errorId) {
        throw new Error(
          "expected submit work request name aria-describedby to reference the validation message",
        );
      }
    },
    await requestError.getAttribute("id"),
  );
  await workType.evaluate(
    (element, errorId) => {
      if (element.getAttribute("aria-invalid") !== "true") {
        throw new Error(
          "expected submit work type combobox to be aria-invalid",
        );
      }
      if (element.getAttribute("aria-describedby") !== errorId) {
        throw new Error(
          "expected submit work type aria-describedby to reference the validation message",
        );
      }
    },
    await workTypeError.getAttribute("id"),
  );

  await expectNoOverflow(
    page,
    `Submit work card validation alignment at ${viewport.label}`,
  );
}
