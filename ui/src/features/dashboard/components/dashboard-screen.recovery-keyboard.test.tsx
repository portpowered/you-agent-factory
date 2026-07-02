import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { useDashboardBentoStore } from "../../bento/state/dashboardBentoStore";
import { getHeaderControlsMessages } from "../../header/messages/header-controls";
import { getDashboardRecoveryMessages } from "../messages/dashboard-recovery";
import { DashboardScreen } from "./dashboard-screen";

let dashboardSnapshotState: ReturnType<
  typeof import("../hooks/useDashboardSnapshot").useDashboardSnapshot
>;

vi.mock("../../bento/public", () => ({
  DashboardBento: () => <section>Dashboard bento</section>,
}));

vi.mock("../../header/public", () => ({
  DashboardExportDialog: () => <div>Dashboard export dialog</div>,
  DashboardHeader: () => <header>Dashboard header</header>,
  DashboardStatusPanel: ({
    detail,
    title,
  }: {
    detail?: string;
    title: string;
  }) => (
    <section>
      <h1>{title}</h1>
      {detail ? <p>{detail}</p> : null}
    </section>
  ),
}));

vi.mock("../hooks/useDashboardSnapshot", () => ({
  useDashboardSnapshot: vi.fn(() => dashboardSnapshotState),
}));

describe("DashboardScreen recovery keyboard access", () => {
  beforeEach(() => {
    useDashboardBentoStore.setState({
      refreshToken: 0,
      selectedTraceID: null,
    });
    dashboardSnapshotState = {
      error: new Error(
        "The dashboard could not restore this session automatically.",
      ),
      isInitialLoading: false,
      preflightRecovery: null,
      preflightStatus: "success",
      snapshot: null,
      streamState: {
        message:
          "The dashboard could not restore this session automatically.",
        status: "recovery_failed",
      },
    };
  });

  it("keeps replay recovery actions keyboard reachable without generic offline chrome", async () => {
    const user = userEvent.setup();
    const messages = getHeaderControlsMessages("en");
    const recoveryMessages = getDashboardRecoveryMessages("en");

    render(<DashboardScreen />);

    expect(
      screen.getByRole("heading", {
        name: recoveryMessages.recoveryFailedTitle,
      }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("heading", { name: messages.dashboardUnavailableTitle }),
    ).toBeNull();

    const retryButton = screen.getByRole("button", {
      name: recoveryMessages.recoveryFailedRetryLabel,
    });
    retryButton.focus();
    expect(document.activeElement).toBe(retryButton);
    await user.keyboard("{Enter}");
    expect(useDashboardBentoStore.getState().refreshToken).toBe(1);
  });
});
