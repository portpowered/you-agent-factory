export const STYLED_CHECKBOX_INPUT_MARKER = "sr-only";

export const STYLED_CHECKBOX_INDICATOR_MARKERS = [
  "peer-checked:bg-primary",
  "border-outline",
  "peer-focus-visible:ring-af-focus-ring",
  "peer-disabled:bg-surface-container-low",
  "peer-aria-invalid:ring-af-danger-border",
];

export async function readCheckboxIndicatorClassName(checkboxLocator) {
  const indicator = checkboxLocator.locator("xpath=following-sibling::*[1]");
  await indicator.waitFor({ state: "attached" });
  const className = await indicator.evaluate((element) => element.className);
  if (typeof className !== "string" || className.length === 0) {
    throw new Error("Could not read styled checkbox indicator className.");
  }

  return className;
}

export async function assertStyledCheckboxTreatment(checkboxLocator, label) {
  const inputClassName = await checkboxLocator.evaluate(
    (element) => element.className,
  );
  if (
    typeof inputClassName !== "string" ||
    !inputClassName.includes(STYLED_CHECKBOX_INPUT_MARKER)
  ) {
    throw new Error(
      `${label}: checkbox input is missing ${STYLED_CHECKBOX_INPUT_MARKER} styling.`,
    );
  }

  const indicatorClassName =
    await readCheckboxIndicatorClassName(checkboxLocator);
  for (const marker of STYLED_CHECKBOX_INDICATOR_MARKERS) {
    if (!indicatorClassName.includes(marker)) {
      throw new Error(
        `${label}: checkbox indicator is missing ${marker} styling.`,
      );
    }
  }

  const indicator = checkboxLocator.locator("xpath=following-sibling::*[1]");
  const svgCount = await indicator.locator("svg").count();
  if (svgCount === 0) {
    throw new Error(
      `${label}: checkbox indicator is missing the checked mark.`,
    );
  }
}

export async function assertCheckboxCheckedState(
  checkboxLocator,
  expectedChecked,
  label,
) {
  const checked = await checkboxLocator.isChecked();
  if (checked !== expectedChecked) {
    throw new Error(
      `${label}: expected checked=${expectedChecked}, received checked=${checked}.`,
    );
  }
}

export async function assertCheckboxInvalidState(checkboxLocator, label) {
  const ariaInvalid = await checkboxLocator.getAttribute("aria-invalid");
  if (ariaInvalid !== "true") {
    throw new Error(`${label}: expected aria-invalid="true".`);
  }

  const indicatorClassName =
    await readCheckboxIndicatorClassName(checkboxLocator);
  if (!indicatorClassName.includes("peer-aria-invalid:ring-af-danger-border")) {
    throw new Error(`${label}: invalid checkbox indicator styling is missing.`);
  }
}

export async function assertCheckboxDisabledState(checkboxLocator, label) {
  if (!(await checkboxLocator.isDisabled())) {
    throw new Error(`${label}: expected the checkbox to be disabled.`);
  }

  const indicatorClassName =
    await readCheckboxIndicatorClassName(checkboxLocator);
  if (!indicatorClassName.includes("peer-disabled:bg-surface-container-low")) {
    throw new Error(
      `${label}: disabled checkbox indicator styling is missing.`,
    );
  }
}

export async function toggleCheckboxFromLabel(page, labelText) {
  const label = page.getByText(labelText, { exact: true });
  await label.waitFor({ state: "visible" });
  await label.click();
}

export async function toggleCheckboxWithSpace(page, checkboxLocator) {
  await checkboxLocator.focus();
  await page.keyboard.press("Space");
}
