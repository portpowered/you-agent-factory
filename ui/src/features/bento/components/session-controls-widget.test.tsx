import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { SessionControlsWidget } from "./session-controls-widget";

const sessionTabsState = {
  activeSession: {
    factoryDir: "/workspace/factory",
    folderPath: "/workspace/factory",
    id: "~default",
    isDefault: true,
    project: "factory",
    target: { kind: "default" as const },
  },
  isSessionStreamPaused: () => false,
  toggleSessionStreamPaused: vi.fn(),
};

vi.mock("../../export/state/exportDialogStore", () => ({
  useExportDialogStore: (
    selector: (state: {
      isExportDialogOpen: boolean;
      openExportDialog: () => void;
    }) => unknown,
  ) =>
    selector({
      isExportDialogOpen: false,
      openExportDialog: vi.fn(),
    }),
}));

vi.mock("../../header/hooks/use-dashboard-session-tabs-state", () => ({
  useDashboardSessionTabsState: () => sessionTabsState,
}));

vi.mock("../../header/components/tick-slider-control", () => ({
  TickSliderControl: () => <input aria-label="Timeline tick" type="range" />,
}));

describe("SessionControlsWidget", () => {
  it("renders timeline actions in portable bento card chrome with body inset", () => {
    render(<SessionControlsWidget locale="en" />);

    const card = screen.getByRole("article", { name: "Session controls" });
    const cardHeader = card.querySelector("header");
    const cardBody = card.querySelector(".pb-4");

    expect(card.dataset.dashboardPanelShell).toBe("grid-card");
    expect(cardHeader?.getAttribute("data-bento-drag-handle")).toBe("true");
    expect(cardBody?.className).toContain("pt-3.5");
    expect(cardBody?.className).toContain("pb-4");
    expect(screen.getByRole("slider", { name: "Timeline tick" })).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Pause factory updates" }),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: "Export PNG" })).toBeTruthy();
  });
});
