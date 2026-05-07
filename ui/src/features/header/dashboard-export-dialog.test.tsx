import { fireEvent, render, screen, within } from "@testing-library/react";

import type { FactoryValue } from "../../api/named-factory";
import { getExportDialogMessages } from "../export/messages/export-dialog";
import { DashboardExportDialog } from "./dashboard-export-dialog";

const closeExportDialog = vi.fn();
let isExportDialogOpen = false;
let currentFactoryExportState: ReturnType<
  typeof import("../export").useCurrentFactoryExport
>;

vi.mock("../export/state/exportDialogStore", () => ({
  useExportDialogStore: (
    selector: (state: {
      closeExportDialog: () => void;
      isExportDialogOpen: boolean;
      openExportDialog: () => void;
    }) => unknown,
  ) =>
    selector({
      closeExportDialog,
      isExportDialogOpen,
      openExportDialog: vi.fn(),
    }),
}));

vi.mock("../export", async () => {
  const actual = await vi.importActual("../export");

  return {
    ...actual,
    useCurrentFactoryExport: vi.fn(() => currentFactoryExportState),
  };
});

const factory = {
  name: "Factory Aurora",
  workspaces: {},
} as const satisfies Partial<FactoryValue>;

describe("DashboardExportDialog", () => {
  beforeEach(() => {
    closeExportDialog.mockReset();
    isExportDialogOpen = false;
    currentFactoryExportState = {
      currentFactoryExport: {
        factoryDefinition: factory as FactoryValue,
        ok: true,
      },
      isPreparing: false,
    };
  });

  it("does not render the export dialog host while the dashboard store is closed", () => {
    render(<DashboardExportDialog />);

    expect(
      screen.queryByRole("dialog", {
        name: getExportDialogMessages("en").title,
      }),
    ).toBeNull();
  });

  it("renders the dashboard-owned export dialog and closes it through shared dialog controls", async () => {
    isExportDialogOpen = true;
    const messages = getExportDialogMessages("en");

    render(<DashboardExportDialog />);

    const dialog = await screen.findByRole("dialog", { name: messages.title });
    expect(
      within(dialog).getByRole("button", { name: messages.cancelAction }),
    ).toBeTruthy();
    expect(
      within(dialog).getByRole("button", { name: messages.exportAction }),
    ).toBeTruthy();

    fireEvent.click(
      within(dialog).getByRole("button", { name: messages.closeLabel }),
    );

    expect(closeExportDialog).toHaveBeenCalledTimes(1);
  });

  it("keeps the export action disabled and shows preparation feedback while the current factory loads", async () => {
    isExportDialogOpen = true;
    currentFactoryExportState = {
      currentFactoryExport: {
        code: "FACTORY_DEFINITION_UNAVAILABLE",
        message: "The current factory definition is not available yet.",
        ok: false,
      },
      isPreparing: true,
    };
    const messages = getExportDialogMessages("en");

    render(<DashboardExportDialog />);

    const dialog = await screen.findByRole("dialog", { name: messages.title });
    expect(within(dialog).getByText(messages.loadingStatus)).toBeTruthy();
    expect(
      (
        within(dialog).getByRole("button", {
          name: messages.exportAction,
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
  });

  it("shows the preparation failure message when the current factory cannot be prepared", async () => {
    isExportDialogOpen = true;
    currentFactoryExportState = {
      currentFactoryExport: {
        code: "FACTORY_DEFINITION_UNAVAILABLE",
        message:
          "The current factory definition could not be loaded from the current-factory API.",
        ok: false,
      },
      isPreparing: false,
    };
    const messages = getExportDialogMessages("en");

    render(<DashboardExportDialog />);

    const dialog = await screen.findByRole("dialog", { name: messages.title });
    expect(
      within(dialog).getByText(
        "The current factory definition could not be loaded from the current-factory API.",
      ),
    ).toBeTruthy();
    expect(
      within(dialog).getByRole("button", { name: messages.cancelAction }),
    ).toBeTruthy();
  });

  it("falls back to the Infinite You filename slug when the current factory is unavailable", async () => {
    isExportDialogOpen = true;
    currentFactoryExportState = {
      currentFactoryExport: {
        code: "FACTORY_DEFINITION_UNAVAILABLE",
        message:
          "The current factory definition could not be loaded from the current-factory API.",
        ok: false,
      },
      isPreparing: false,
    };
    const messages = getExportDialogMessages("en");

    render(<DashboardExportDialog />);

    const dialog = await screen.findByRole("dialog", { name: messages.title });
    expect(
      (
        within(dialog).getByRole("textbox", {
          name: messages.nameLabel,
        }) as HTMLInputElement
      ).value,
    ).toBe("infinite-you");
  });

  it("renders the localized export dialog surface when a locale is provided", async () => {
    isExportDialogOpen = true;
    const messages = getExportDialogMessages("ja");

    render(<DashboardExportDialog locale="ja" />);

    const dialog = await screen.findByRole("dialog", { name: messages.title });
    expect(within(dialog).getByText(messages.description)).toBeTruthy();
    expect(
      within(dialog).getByRole("button", { name: messages.cancelAction }),
    ).toBeTruthy();
    expect(
      within(dialog).getByRole("button", { name: messages.exportAction }),
    ).toBeTruthy();
    expect(
      within(dialog).getByRole("button", { name: messages.closeLabel }),
    ).toBeTruthy();
  });
});
