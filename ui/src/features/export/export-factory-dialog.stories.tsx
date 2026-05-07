import { expect, fireEvent, userEvent, within } from "storybook/test";

import { ExportFactoryDialog } from "./export-factory-dialog";
import { getExportDialogMessages } from "./messages/export-dialog";

const factory = {
  name: "Factory Aurora",
  workspaces: {},
} as const;

export default {
  title: "Infinite You/Dashboard/Export Factory Dialog",
  component: ExportFactoryDialog,
  tags: ["test"],
};

export const Ready = {
  args: {
    factory,
    initialFactoryName: "Factory Aurora",
    isOpen: true,
    onClose: () => {},
  },
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const messages = getExportDialogMessages("en");
    const canvas = within(canvasElement.ownerDocument.body);
    const dialog = await canvas.findByRole("dialog", { name: messages.title });
    const scope = within(dialog);
    const nameInput = scope.getByRole("textbox", { name: messages.nameLabel });
    const imageInput = scope.getByLabelText(messages.imageLabel);
    const cancelButton = scope.getByRole("button", {
      name: messages.cancelAction,
    });
    const exportButton = scope.getByRole("button", {
      name: messages.exportAction,
    });

    await expect(nameInput).toHaveValue("Factory Aurora");
    await expect(scope.getByText(messages.hint)).toBeVisible();
    await expect(exportButton).toBeEnabled();

    nameInput.focus();
    await expect(nameInput).toHaveFocus();
    await userEvent.tab();
    await expect(imageInput).toHaveFocus();
    await userEvent.tab();
    await expect(cancelButton).toHaveFocus();
    await userEvent.tab();
    await expect(exportButton).toHaveFocus();
  },
};

export const Validation = {
  args: {
    factory,
    initialFactoryName: "",
    isOpen: true,
    onClose: () => {},
  },
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const messages = getExportDialogMessages("en");
    const canvas = within(canvasElement.ownerDocument.body);
    const dialog = await canvas.findByRole("dialog", { name: messages.title });
    const scope = within(dialog);

    fireEvent.click(scope.getByRole("button", { name: messages.exportAction }));
    await expect(
      scope.getByText(messages.nameRequiredValidation),
    ).toBeVisible();
    await expect(
      scope.getByText(messages.imageRequiredValidation),
    ).toBeVisible();
  },
};

export const Preparing = {
  args: {
    factory,
    initialFactoryName: "Factory Aurora",
    isOpen: true,
    isPreparing: true,
    onClose: () => {},
  },
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const messages = getExportDialogMessages("en");
    const canvas = within(canvasElement.ownerDocument.body);
    const dialog = await canvas.findByRole("dialog", { name: messages.title });
    const scope = within(dialog);

    await expect(
      scope.getByRole("button", { name: messages.exportAction }),
    ).toBeDisabled();
    await expect(scope.getByText(messages.loadingStatus)).toBeVisible();
  },
};

export const LocalizedJa = {
  args: {
    factory,
    initialFactoryName: "Factory Aurora",
    isOpen: true,
    locale: "ja",
    onClose: () => {},
  },
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const messages = getExportDialogMessages("ja");
    const canvas = within(canvasElement.ownerDocument.body);
    const dialog = await canvas.findByRole("dialog", { name: messages.title });
    const scope = within(dialog);

    await expect(scope.getByText(messages.description)).toBeVisible();
    await expect(scope.getByText(messages.hint)).toBeVisible();
    await expect(
      scope.getByRole("textbox", { name: messages.nameLabel }),
    ).toHaveValue("Factory Aurora");
    await expect(scope.getByLabelText(messages.imageLabel)).toBeVisible();
    await expect(
      scope.getByRole("button", { name: messages.cancelAction }),
    ).toBeVisible();
    await expect(
      scope.getByRole("button", { name: messages.exportAction }),
    ).toBeVisible();
  },
};
