import { expect, fireEvent, userEvent, within } from "storybook/test";
import { getExportDialogMessages } from "../messages/export-dialog";
import { ExportFactoryDialog } from "./export-factory-dialog";

const factory = {
  name: "Factory Aurora",
  workspaces: {},
} as const;

export default {
  title: "you-agent-factory/Dashboard/Export Factory Dialog",
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

export const UnifiedChooseFileChrome = {
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
    const imageInput = scope.getByLabelText(messages.imageLabel);

    await expect(imageInput.className).toContain("border-dashed");
    await expect(imageInput.className).toContain("border-af-border-strong");
    await expect(imageInput.className).toContain("bg-af-surface-subtle");
    await expect(imageInput.className).not.toContain("bg-af-accent-surface");
    await expect(imageInput.className).not.toContain("border-af-accent-border");
    await expect(imageInput.className).not.toContain(
      "file:bg-af-accent-surface",
    );
    await expect(imageInput.className).not.toContain("file:text-af-accent");

    const coverImage = new File(["png"], "cover.png", { type: "image/png" });
    await userEvent.upload(imageInput, coverImage);
    await expect(
      scope.getByText(messages.selectedImageLabel("cover.png")),
    ).toBeVisible();
  },
};

export const LocalizedZhCn = {
  args: {
    factory,
    initialFactoryName: "Factory Aurora",
    isOpen: true,
    locale: "zh-CN",
    onClose: () => {},
  },
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const messages = getExportDialogMessages("zh-CN");
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
