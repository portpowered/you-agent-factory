import { expect, userEvent, within } from "storybook/test";

import { UIFoundationShowcase } from "./ui-foundation-showcase";

export default {
  title: "Agent Factory/UI/Shared Primitive Foundation",
  component: UIFoundationShowcase,
  tags: ["test"],
};

export const Default = {
  render: () => <UIFoundationShowcase />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const page = within(canvasElement.ownerDocument.body);

    await expect(canvas.getByRole("button", { name: "Primary action" })).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Disabled action" })).toBeDisabled();
    await expect(canvas.getByRole("textbox", { name: "Request name" })).toBeVisible();
    await expect(canvas.getByRole("textbox", { name: "Request text" })).toBeVisible();
    await expect(canvas.getByRole("combobox", { name: "Work type" })).toBeVisible();
    await expect(canvas.getByRole("img", { name: "Primitive chart showcase" })).toBeVisible();
    await expect(
      canvas.getByRole("table", { name: "Primitive table foundation for trace and detail surfaces." }),
    ).toBeVisible();
    await expect(canvas.getByRole("table", { name: "Primitive data table showcase" })).toBeVisible();
    await expect(canvas.getByLabelText("Primitive calendar showcase")).toBeVisible();
    await expect(canvas.getByText("Sidebar panel")).toBeVisible();
    await expect(canvas.getByText("Detail panel")).toBeVisible();

    const collapseButton = canvas.getByRole("button", { name: "Collapse" });
    await userEvent.click(collapseButton);
    await expect(collapseButton).toHaveAttribute("aria-expanded", "false");
    await userEvent.click(canvas.getByRole("button", { name: "Expand" }));

    await userEvent.click(canvas.getByRole("button", { name: "Open dialog" }));
    const dialog = await page.findByRole("dialog", { name: "Export factory" });
    await expect(within(dialog).getByRole("button", { name: "Confirm export" })).toBeVisible();
    await userEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));

    await userEvent.tab();
    await expect(canvas.getByRole("button", { name: "Primary action" })).toHaveFocus();
  },
};

