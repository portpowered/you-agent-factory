import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { FactorySessionDetailPanel } from "../factory-session-detail-panel";
import {
  BASELINE_DISPATCH_ID,
  BASELINE_SESSION_ID,
  mockJavaScriptSessionBetaFetchWithDeferredDispatch,
} from "../test-support/factory-session-detail-panel.baseline-fixtures";
import {
  createBaselineDispatchDetailPayload,
  DISPATCH_API_ERROR_ID,
  DISPATCH_NOT_FOUND_ID,
  DISPATCH_REPLACEMENT_ALPHA_ID,
  DISPATCH_REPLACEMENT_BETA_ID,
  mockDispatchApiErrorFetch,
  mockDispatchNotFoundFetch,
  mockDispatchReplacementFetch,
} from "../test-support/factory-session-detail-panel.dispatch-drilldown-fixtures";
import {
  jsonResponse,
  renderWithQueryClient,
} from "../test-support/factory-session-detail-panel.test-helpers";

describe("FactorySessionDetailPanel dispatch drilldown loading and unavailable states", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("shows loading and success states for a JavaScript factory session dispatch drilldown", async () => {
    const dispatchDetailResponse =
      mockJavaScriptSessionBetaFetchWithDeferredDispatch();

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID={BASELINE_SESSION_ID} />,
    );

    await waitFor(() => {
      expect(screen.getByText("Runtime")).toBeTruthy();
    });

    const user = userEvent.setup();
    const drilldownTrigger = screen.getByRole("button", {
      name: `Expand dispatch detail for ${BASELINE_DISPATCH_ID}`,
    });

    expect(drilldownTrigger.getAttribute("aria-expanded")).toBe("false");

    await user.click(drilldownTrigger);

    expect(screen.getByText("Loading dispatch detail…")).toBeTruthy();
    expect(drilldownTrigger.getAttribute("aria-expanded")).toBe("true");

    dispatchDetailResponse.resolve(
      jsonResponse(createBaselineDispatchDetailPayload()),
    );

    await waitFor(() => {
      expect(screen.getByText("JavaScript task")).toBeTruthy();
    });

    expect(screen.getAllByText("COMPLETED").length).toBeGreaterThan(0);
    expect(screen.getAllByText("JAVASCRIPT_AGENT").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Review child task").length).toBeGreaterThan(1);
    expect(screen.getByText("live")).toBeTruthy();
    expect(screen.getByText("session_id · provider-session-1")).toBeTruthy();
    expect(
      screen.getByRole("link", { name: "artifact-dispatch-1" }),
    ).toBeTruthy();
    expect(screen.getByText("QUEUED")).toBeTruthy();
    expect(screen.getByText("RUNNING")).toBeTruthy();
    expect(screen.getByText("$0.21")).toBeTruthy();
    expect(screen.getByText("4,400 ms")).toBeTruthy();
    expect(screen.getByText("Token budget was nearly exhausted.")).toBeTruthy();
  });

  it("shows unavailable dispatch detail when the durable dispatch read returns not found", async () => {
    mockDispatchNotFoundFetch();

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID={BASELINE_SESSION_ID} />,
    );

    await waitFor(() => {
      expect(screen.getByText("Dispatches")).toBeTruthy();
    });

    const user = userEvent.setup();
    const drilldownTrigger = screen.getByRole("button", {
      name: `Expand dispatch detail for ${DISPATCH_NOT_FOUND_ID}`,
    });
    drilldownTrigger.focus();
    await user.keyboard("{Enter}");

    await waitFor(() => {
      expect(
        screen.getByText(
          `Dispatch detail for ${DISPATCH_NOT_FOUND_ID} is no longer available.`,
        ),
      ).toBeTruthy();
    });
    expect(screen.getByText("Dispatches")).toBeTruthy();
    expect(screen.getByText("Runtime")).toBeTruthy();
  });
});

describe("FactorySessionDetailPanel dispatch drilldown replacement and error states", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("replaces dispatch detail when selecting a different dispatch summary row", async () => {
    mockDispatchReplacementFetch();

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID={BASELINE_SESSION_ID} />,
    );

    await waitFor(() => {
      expect(screen.getByText("Runtime")).toBeTruthy();
    });

    expect(screen.getByText("JavaScript workflow")).toBeTruthy();
    expect(screen.getByText("verify")).toBeTruthy();

    const user = userEvent.setup();
    const alphaTrigger = screen.getByRole("button", {
      name: `Expand dispatch detail for ${DISPATCH_REPLACEMENT_ALPHA_ID}`,
    });
    const betaTrigger = screen.getByRole("button", {
      name: `Expand dispatch detail for ${DISPATCH_REPLACEMENT_BETA_ID}`,
    });

    expect(alphaTrigger.getAttribute("aria-expanded")).toBe("false");
    expect(betaTrigger.getAttribute("aria-expanded")).toBe("false");

    await user.click(alphaTrigger);

    await waitFor(() => {
      expect(screen.getByRole("link", { name: "artifact-alpha" })).toBeTruthy();
    });
    expect(screen.getAllByText("COMPLETED").length).toBeGreaterThan(0);
    expect(alphaTrigger.getAttribute("aria-expanded")).toBe("true");
    expect(betaTrigger.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByRole("link", { name: "artifact-beta" })).toBeNull();
    expect(screen.queryByText("Checksum mismatch on beta verify.")).toBeNull();

    await user.click(betaTrigger);

    await waitFor(() => {
      expect(screen.getByRole("link", { name: "artifact-beta" })).toBeTruthy();
    });
    expect(screen.getByText("Checksum mismatch on beta verify.")).toBeTruthy();
    expect(alphaTrigger.getAttribute("aria-expanded")).toBe("false");
    expect(betaTrigger.getAttribute("aria-expanded")).toBe("true");
    expect(screen.queryByRole("link", { name: "artifact-alpha" })).toBeNull();
    expect(screen.getByText("Runtime")).toBeTruthy();
    expect(screen.getByText("JavaScript workflow")).toBeTruthy();
  });

  it("shows a dispatch-detail error state when the durable dispatch read fails", async () => {
    mockDispatchApiErrorFetch();

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID={BASELINE_SESSION_ID} />,
    );

    await waitFor(() => {
      expect(screen.getByText("Dispatches")).toBeTruthy();
    });

    const user = userEvent.setup();
    await user.click(
      screen.getByRole("button", {
        name: `Expand dispatch detail for ${DISPATCH_API_ERROR_ID}`,
      }),
    );

    await waitFor(() => {
      expect(screen.getByText("dispatch boom")).toBeTruthy();
    });

    expect(screen.queryByText("Loading factory session runtime…")).toBeNull();
    expect(
      screen.queryByText("This factory session is no longer available."),
    ).toBeNull();
    expect(
      screen.getByRole("button", { name: "Retry loading dispatch detail" }),
    ).toBeTruthy();
    expect(screen.getByText("Dispatches")).toBeTruthy();
    expect(screen.getByText("Runtime")).toBeTruthy();
  });
});
