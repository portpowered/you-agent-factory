import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { FactorySessionDetailPanel } from "../factory-session-detail-panel";
import {
  DISPATCH_DETAIL_SESSION_ID,
  DISPATCH_ERROR_ID,
  DISPATCH_FAILED_ID,
  DISPATCH_MISSING_ID,
  DISPATCH_SUCCESS_ID,
  DISPATCH_WARNING_ID,
  mockDispatchDetailBoundaryFetch,
  mockFailedDispatchDetailFetch,
  mockSuccessfulDispatchDetailFetch,
  mockWarningDispatchDetailFetch,
  PRIMARY_PROVIDER_SESSION,
} from "../test-support/factory-session-detail-panel.dispatch-detail-fixtures";
import { renderWithQueryClient } from "../test-support/factory-session-detail-panel.test-helpers";

function expectSummaryState() {
  expect(screen.getAllByText("Execution mode: live").length).toBeGreaterThan(0);
  expect(screen.getByText(PRIMARY_PROVIDER_SESSION)).toBeTruthy();
  expect(screen.getByText("artifact-final · FINAL_RESULT")).toBeTruthy();
}

describe("FactorySessionDetailPanel dispatch detail failure payload", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("renders successful dispatch detail with status, execution mode, provider sessions, and artifacts", async () => {
    mockSuccessfulDispatchDetailFetch();

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID={DISPATCH_DETAIL_SESSION_ID} />,
    );

    await waitFor(() => {
      expect(screen.getByText("Dispatches")).toBeTruthy();
    });

    const user = userEvent.setup();
    await user.click(
      screen.getByRole("button", {
        name: `Expand dispatch detail for ${DISPATCH_SUCCESS_ID}`,
      }),
    );

    await waitFor(() => {
      expect(screen.getByText("JavaScript task")).toBeTruthy();
    });

    expect(screen.getAllByText("COMPLETED").length).toBeGreaterThan(0);
    expect(screen.getByText("live")).toBeTruthy();
    expect(screen.getByText("Provider sessions")).toBeTruthy();
    expect(screen.getByText("session_id · sess_codex_1")).toBeTruthy();
    expect(screen.getByRole("link", { name: "artifact-final-1" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "artifact-log-2" })).toBeTruthy();
    expect(screen.getByText("Runtime")).toBeTruthy();
    expect(screen.getByText("JavaScript workflow")).toBeTruthy();
  });

  it("renders failed dispatch detail with typed failure data and artifact links", async () => {
    mockFailedDispatchDetailFetch();

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID={DISPATCH_DETAIL_SESSION_ID} />,
    );

    await waitFor(() => {
      expect(screen.getByText("Dispatches")).toBeTruthy();
    });

    const user = userEvent.setup();
    await user.click(
      screen.getByRole("button", {
        name: `Expand dispatch detail for ${DISPATCH_FAILED_ID}`,
      }),
    );

    await waitFor(() => {
      expect(screen.getByText("Failure detail")).toBeTruthy();
    });

    expect(screen.getByText("VERIFY_ASSERTION_FAILED")).toBeTruthy();
    expect(
      screen.getByText("Expected release manifest checksum."),
    ).toBeTruthy();
    expect(
      screen.getByRole("link", { name: "artifact-failure-log" }),
    ).toBeTruthy();
    expect(screen.getAllByText("FAILED").length).toBeGreaterThan(1);
    expect(screen.getByText("Failure detail")).toBeTruthy();
  });

  it("renders warning dispatch detail with typed warning data", async () => {
    mockWarningDispatchDetailFetch();

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID={DISPATCH_DETAIL_SESSION_ID} />,
    );

    await waitFor(() => {
      expect(screen.getByText("Dispatches")).toBeTruthy();
    });

    const user = userEvent.setup();
    await user.click(
      screen.getByRole("button", {
        name: `Expand dispatch detail for ${DISPATCH_WARNING_ID}`,
      }),
    );

    await waitFor(() => {
      expect(screen.getByText("DISPATCH_WARNING")).toBeTruthy();
    });

    expect(
      screen.getAllByText("Dispatch warnings").length,
    ).toBeGreaterThanOrEqual(1);
    expect(
      screen.getAllByText("Verification completed with non-blocking warnings.")
        .length,
    ).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("DISPATCH_WARNING")).toBeTruthy();
    expect(screen.queryByText("Failure detail")).toBeNull();
    expect(screen.getByText("Runtime")).toBeTruthy();
  });
});

describe("FactorySessionDetailPanel dispatch detail boundary states", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("preserves live-provider summary details while dispatch detail hits missing and error states", async () => {
    mockDispatchDetailBoundaryFetch();

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID={DISPATCH_DETAIL_SESSION_ID} />,
    );

    await waitFor(() => {
      expect(screen.getByText("Dispatches")).toBeTruthy();
    });

    expectSummaryState();
    expect(screen.getByText("Token budget was nearly exhausted.")).toBeTruthy();

    const user = userEvent.setup();
    await user.click(
      screen.getByRole("button", {
        name: `Expand dispatch detail for ${DISPATCH_MISSING_ID}`,
      }),
    );

    await waitFor(() => {
      expect(
        screen.getByText(
          `Dispatch detail for ${DISPATCH_MISSING_ID} is no longer available.`,
        ),
      ).toBeTruthy();
    });

    expectSummaryState();

    await user.click(
      screen.getByRole("button", {
        name: `Expand dispatch detail for ${DISPATCH_ERROR_ID}`,
      }),
    );

    await waitFor(() => {
      expect(screen.getByText("dispatch boom")).toBeTruthy();
    });

    expectSummaryState();
  });
});
