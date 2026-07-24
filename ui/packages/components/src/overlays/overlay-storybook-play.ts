import { expect, userEvent, within } from "storybook/test";

export async function verifyDialogKeyboardFocus({
  canvasElement,
}: {
  canvasElement: HTMLElement;
}) {
  const canvas = within(canvasElement);
  const page = within(canvasElement.ownerDocument.body);

  const trigger = canvas.getByRole("button", { name: "Open dialog" });
  await userEvent.click(trigger);

  const dialog = await page.findByRole("dialog", { name: "Package dialog" });
  expect(dialog).toBeVisible();
  expect(within(dialog).getByRole("button", { name: "Close" })).toHaveFocus();

  await userEvent.keyboard("{Escape}");
  await expect(
    page.queryByRole("dialog", { name: "Package dialog" }),
  ).toBeNull();
  await expect(trigger).toHaveFocus();
}

export async function verifyDialogEscapeClose({
  canvasElement,
}: {
  canvasElement: HTMLElement;
}) {
  const canvas = within(canvasElement);
  const page = within(canvasElement.ownerDocument.body);

  const trigger = canvas.getByRole("button", {
    name: "Open dialog for Escape",
  });
  await userEvent.click(trigger);

  const dialog = await page.findByRole("dialog", {
    name: "Escape close dialog",
  });
  expect(dialog).toBeVisible();

  await userEvent.keyboard("{Escape}");
  await expect(
    page.queryByRole("dialog", { name: "Escape close dialog" }),
  ).toBeNull();
  await expect(trigger).toHaveFocus();
}

export async function verifyPopoverKeyboardFocus({
  canvasElement,
}: {
  canvasElement: HTMLElement;
}) {
  const canvas = within(canvasElement);
  const page = within(canvasElement.ownerDocument.body);

  const trigger = canvas.getByRole("button", { name: "Open popover" });
  await userEvent.click(trigger);

  await expect(
    page.getByText("Popover content from the component package."),
  ).toBeVisible();

  await userEvent.keyboard("{Escape}");
  await expect(
    page.queryByText("Popover content from the component package."),
  ).toBeNull();
  await expect(trigger).toHaveFocus();
}

export async function verifyPopoverKeyboardOpen({
  canvasElement,
}: {
  canvasElement: HTMLElement;
}) {
  const canvas = within(canvasElement);
  const page = within(canvasElement.ownerDocument.body);

  const trigger = canvas.getByRole("button", {
    name: "Open popover with keyboard",
  });
  trigger.focus();
  await userEvent.keyboard("{Enter}");

  await expect(
    page.getByText("Popover opened from the keyboard trigger."),
  ).toBeVisible();
}

export async function verifyCollapsibleKeyboardFocus({
  canvasElement,
}: {
  canvasElement: HTMLElement;
}) {
  const canvas = within(canvasElement);
  const trigger = canvas.getByRole("button", { name: "Toggle details" });

  await expect(trigger).toHaveAttribute("aria-expanded", "false");
  trigger.focus();
  await userEvent.keyboard("{Enter}");
  await expect(trigger).toHaveAttribute("aria-expanded", "true");
  await expect(
    canvas.getByText(
      "Collapsible content rendered from the package overlays category.",
    ),
  ).toBeVisible();
}

export async function verifyScrollAreaKeyboardFocus({
  canvasElement,
}: {
  canvasElement: HTMLElement;
}) {
  const canvas = within(canvasElement);
  const field = canvas.getByRole("textbox", { name: "Scrollable field" });

  await userEvent.tab();
  await expect(field).toHaveFocus();
}
