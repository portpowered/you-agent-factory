import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { FactorySessionWidget } from "./factory-session-widget";

vi.mock("./factory-session-detail-panel", () => ({
  FactorySessionDetailPanel: ({ sessionID }: { sessionID: string | null }) => (
    <div>Selected factory session {sessionID}</div>
  ),
}));

describe("FactorySessionWidget", () => {
  afterEach(() => {
    window.history.replaceState({}, "", "/");
  });

  it("prefers the dashboard factory-session URL override when present", () => {
    window.history.replaceState(
      {},
      "",
      "/dashboard/ui/?factorySessionId=dur-sess-js-success-002",
    );

    render(<FactorySessionWidget sessionID="~default" />);

    expect(
      screen.getByText("Selected factory session dur-sess-js-success-002"),
    ).toBeTruthy();
  });

  it("falls back to the live selected session when the dashboard URL has no override", () => {
    render(<FactorySessionWidget sessionID="session-beta" />);

    expect(
      screen.getByText("Selected factory session session-beta"),
    ).toBeTruthy();
  });
});
