import "../../../../testing/vitest-dom-capabilities.setup";

import { describe, expect, it } from "bun:test";
import { render, screen } from "@testing-library/react";

import { DashboardCardStateBanner } from "./dashboard-card-state-banner";

describe("DashboardCardStateBanner", () => {
  it("renders known-empty, reconnecting, stale, recovering, historical, and live vocabulary", () => {
    const { rerender } = render(
      <DashboardCardStateBanner
        state={{
          content: "known-empty",
          freshness: "fresh",
          temporal: "live",
        }}
      />,
    );

    expect(
      screen.getByRole("status", {
        name: "Card state: No data yet. Live.",
      }),
    ).toBeTruthy();
    expect(screen.getByText("No data yet")).toBeTruthy();
    expect(screen.getByText("Live")).toBeTruthy();

    rerender(
      <DashboardCardStateBanner
        state={{
          content: "populated",
          freshness: "reconnecting",
          temporal: "live",
        }}
      />,
    );
    expect(screen.getByText("Reconnecting")).toBeTruthy();
    expect(screen.getByText("Live")).toBeTruthy();

    rerender(
      <DashboardCardStateBanner
        state={{
          content: "populated",
          freshness: "stale",
          temporal: "historical",
        }}
      />,
    );
    expect(screen.getByText("Stale data")).toBeTruthy();
    expect(screen.getByText("Historical")).toBeTruthy();

    rerender(
      <DashboardCardStateBanner
        state={{
          content: "loading",
          freshness: "recovering",
          temporal: "historical",
        }}
      />,
    );
    expect(screen.getByText("Recovering")).toBeTruthy();
    expect(screen.getByText("Historical")).toBeTruthy();
  });
});
