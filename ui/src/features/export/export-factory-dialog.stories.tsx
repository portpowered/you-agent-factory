import { expect, fireEvent, within } from "storybook/test";

import { ExportFactoryDialog } from "./export-factory-dialog";

const namedFactory = {
  factory: {
    workspaces: {},
  },
  name: "Factory Aurora",
} as const;

export default {
  title: "Agent Factory/Dashboard/Export Factory Dialog",
  component: ExportFactoryDialog,
};

export const Ready = {
  args: {
    initialFactoryName: "Factory Aurora",
    isOpen: true,
    namedFactory,
    onClose: () => {},
  },
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement.ownerDocument.body);
    const dialog = await canvas.findByRole("dialog", { name: "Export factory" });
    const scope = within(dialog);

    await expect(scope.getByRole("textbox", { name: "Factory name" })).toHaveValue(
      "Factory Aurora",
    );
    await expect(scope.getByRole("button", { name: "Export PNG" })).toBeEnabled();
  },
};

export const Validation = {
  args: {
    initialFactoryName: "",
    isOpen: true,
    namedFactory,
    onClose: () => {},
  },
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement.ownerDocument.body);
    const dialog = await canvas.findByRole("dialog", { name: "Export factory" });
    const scope = within(dialog);

    fireEvent.click(scope.getByRole("button", { name: "Export PNG" }));
    await expect(scope.getByText("Enter a factory name before exporting.")).toBeVisible();
    await expect(scope.getByText("Choose a cover image before exporting.")).toBeVisible();
  },
};

export const Preparing = {
  args: {
    initialFactoryName: "Factory Aurora",
    isOpen: true,
    isPreparing: true,
    namedFactory,
    onClose: () => {},
  },
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement.ownerDocument.body);
    const dialog = await canvas.findByRole("dialog", { name: "Export factory" });
    const scope = within(dialog);

    await expect(scope.getByRole("button", { name: "Export PNG" })).toBeDisabled();
    await expect(scope.getByText("Loading the current authored factory definition.")).toBeVisible();
  },
};
