import { screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { FactorySessionDetailPanel } from "./factory-session-detail-panel";
import {
  BASELINE_DISPATCH_ID,
  BASELINE_SESSION_ID,
  mockJavaScriptSessionBetaFetch,
  mockPendingSessionFetch,
} from "./test-support/factory-session-detail-panel.baseline-fixtures";
import { renderWithQueryClient } from "./test-support/factory-session-detail-panel.test-helpers";

describe("FactorySessionDetailPanel baseline loading and success", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("shows loading state while factory session runtime data is pending", async () => {
    const pendingFetch = mockPendingSessionFetch();

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID={BASELINE_SESSION_ID} />,
    );

    expect(screen.getByText("Loading factory session runtime…")).toBeTruthy();
    expect(
      screen.queryByText("Dynamic workflow (JavaScript factory session)"),
    ).toBeNull();
    expect(
      screen.queryByText("This factory session is no longer available."),
    ).toBeNull();

    pendingFetch.resolveWithPetriSession();

    await waitFor(() => {
      expect(screen.queryByText("Loading factory session runtime…")).toBeNull();
    });
  });

  it("renders a successful JavaScript factory session with lifecycle, dispatch summary, artifacts, and durable detail sections", async () => {
    mockJavaScriptSessionBetaFetch();

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID={BASELINE_SESSION_ID} />,
    );

    expect(screen.getByRole("status").textContent).toContain(
      "Loading factory session runtime…",
    );

    await waitFor(() => {
      expect(screen.getByText("Runtime")).toBeTruthy();
    });

    expect(screen.getByText("JavaScript workflow")).toBeTruthy();
    expect(screen.getAllByText("Idle").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("review")).toBeTruthy();
    expect(
      screen.getByText("cp-1 (plan) — saved plan checkpoint"),
    ).toBeTruthy();
    expect(screen.getByText("child agent retry scheduled")).toBeTruthy();
    expect(screen.getByText("artifact-final · FINAL_RESULT")).toBeTruthy();
    expect(screen.getByText("artifact-partial · CHILD_RESULT")).toBeTruthy();
    expect(screen.getByText("Execution mode: live")).toBeTruthy();
    expect(
      screen.getByText(
        "Provider session: codex / session_id / provider-session-1",
      ),
    ).toBeTruthy();
    expect(screen.queryByText("rawCheckpointBody")).toBeNull();
    expect(
      screen.getByRole("button", {
        name: `Expand dispatch detail for ${BASELINE_DISPATCH_ID}`,
      }),
    ).toBeTruthy();
  });
});
