import { render, screen } from "@testing-library/react";

import type { FactorySessionSummary } from "../../../api/factory-sessions";
import { getHeaderControlsMessages } from "../messages/header-controls";
import { OpenSessionButton, SessionTabButton } from "./dashboard-session-tab";

function session(overrides: Partial<FactorySessionSummary> = {}) {
  return {
    factoryDir: "/workspace/root",
    folderPath: "/workspace/root",
    id: "~default",
    isDefault: true,
    project: "root",
    target: { kind: "default" as const },
    ...overrides,
  } satisfies FactorySessionSummary;
}

describe("DashboardSessionTab controls", () => {
  it("renders active tab semantics, subtle shell styling, and live indicator", () => {
    render(
      <SessionTabButton
        active
        buttonRef={() => undefined}
        closeDisabled={false}
        controlsID="panel-root"
        messages={getHeaderControlsMessages("en")}
        onClick={() => undefined}
        onClose={() => undefined}
        onKeyDown={() => undefined}
        session={session()}
        streamStatus="live"
        tabID="tab-root"
      />,
    );

    const tab = screen.getByRole("tab", { name: "root" });
    const shell = tab.parentElement;

    expect(tab.getAttribute("aria-selected")).toBe("true");
    expect(tab.getAttribute("tabindex")).toBe("0");
    expect(shell?.className).toContain("bg-surface-container-low");
    expect(shell?.className).toContain("text-on-surface");
    expect(shell?.className).toContain("flex-1");
    expect(shell?.className).toContain("basis-0");
    expect(shell?.className).not.toContain("max-w-72");
    expect(shell?.className).not.toContain("min-w-40");
    expect(screen.getByTestId("dashboard-session-live-ping")).toBeTruthy();
  });

  it("renders inactive tab close state and pending close copy", () => {
    const messages = getHeaderControlsMessages("en");
    render(
      <SessionTabButton
        active={false}
        buttonRef={() => undefined}
        closeDisabled
        controlsID="panel-beta"
        messages={messages}
        onClick={() => undefined}
        onClose={() => undefined}
        onKeyDown={() => undefined}
        session={session({
          id: "session-beta",
          isDefault: false,
          project: "beta",
          target: { kind: "named", name: "beta" },
        })}
        streamStatus="offline"
        tabID="tab-beta"
      />,
    );

    const tab = screen.getByRole("tab", { name: "beta" });
    const close = screen.getByRole("button", {
      name: "Close beta session",
    });

    expect(tab.getAttribute("aria-selected")).toBe("false");
    expect(tab.getAttribute("tabindex")).toBe("-1");
    expect(tab.parentElement?.className).toContain("text-on-surface-variant");
    expect(close.getAttribute("disabled")).toBe("");
    expect(close.textContent).toBe(messages.closingSessionButtonLabel);
  });

  it("renders the open-session affordance as a dialog launcher", () => {
    render(
      <OpenSessionButton
        label="Open another factory session"
        onClick={() => undefined}
      />,
    );

    const button = screen.getByRole("button", {
      name: "Open another factory session",
    });

    expect(button.getAttribute("aria-haspopup")).toBe("dialog");
    expect(button.className).toContain("rounded-xl");
  });
});
