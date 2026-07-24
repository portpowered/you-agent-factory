import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  buildSuccessfulDurableSession,
  buildSuccessfulReplayEventStream,
  successfulReplaySessionID,
} from "../../../../testing/factory-session-event-replay-fixtures";
import {
  buildSuccessfulLiveProviderDispatchDetail,
  buildSuccessfulLiveProviderDispatchList,
  successfulLiveProviderDispatchID,
  successfulLiveProviderSessionRef,
} from "../../../../testing/factory-session-live-provider-inspection-fixtures";
import { FactorySessionDetailPanel } from "../factory-session-detail-panel";
import {
  createDeferred,
  jsonResponse,
  renderWithQueryClient,
} from "../test-support/factory-session-detail-panel.test-helpers";

const PROVIDER_SESSION_SUMMARY = `Provider session: ${successfulLiveProviderSessionRef.provider} / ${successfulLiveProviderSessionRef.kind} / ${successfulLiveProviderSessionRef.id}`;

function mockAdjacentSurfacesFetch(options?: {
  deferDispatchDetail?: ReturnType<typeof createDeferred<Response>>;
  dispatchDetailStatus?: number;
}) {
  vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
    const url = String(input);
    if (url.endsWith(`/factory-sessions/${successfulReplaySessionID}`)) {
      return jsonResponse(
        buildSuccessfulDurableSession(successfulReplaySessionID),
      );
    }
    if (
      url.endsWith(`/factory-sessions/${successfulReplaySessionID}/dispatches`)
    ) {
      return jsonResponse(buildSuccessfulLiveProviderDispatchList());
    }
    if (
      url.endsWith(
        `/factory-sessions/${successfulReplaySessionID}/dispatches/${successfulLiveProviderDispatchID}`,
      )
    ) {
      if (options?.deferDispatchDetail) {
        return options.deferDispatchDetail.promise;
      }
      if (options?.dispatchDetailStatus === 404) {
        return new Response("not found", { status: 404 });
      }
      if (options?.dispatchDetailStatus === 500) {
        return jsonResponse(
          { code: "INTERNAL_ERROR", message: "dispatch boom" },
          500,
        );
      }
      return jsonResponse(buildSuccessfulLiveProviderDispatchDetail());
    }
    if (url.endsWith(`/factory-sessions/${successfulReplaySessionID}/events`)) {
      return new Response(
        buildSuccessfulReplayEventStream(successfulReplaySessionID),
        {
          headers: {
            "Content-Type": "text/event-stream",
          },
          status: 200,
        },
      );
    }
    if (
      url.endsWith(
        `/factory-sessions/${successfulReplaySessionID}/results?mode=final`,
      )
    ) {
      return new Response("not found", { status: 404 });
    }
    if (
      url.endsWith(
        `/factory-sessions/${successfulReplaySessionID}/results?mode=partial`,
      )
    ) {
      return new Response("not found", { status: 404 });
    }
    return new Response("not found", { status: 404 });
  });
}

function expectLiveProviderSummaryState() {
  expect(screen.getByText("JavaScript workflow")).toBeTruthy();
  expect(screen.getAllByText("Succeeded").length).toBeGreaterThanOrEqual(1);
  expect(screen.getByText("art-js-success-001 · FINAL_RESULT")).toBeTruthy();
  expect(screen.getAllByText("Execution mode: live").length).toBeGreaterThan(0);
  expect(screen.getByText(PROVIDER_SESSION_SUMMARY)).toBeTruthy();
}

describe("FactorySessionDetailPanel adjacent surfaces regression", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("keeps summary, dispatch drilldown, artifact refs, and event replay when live-provider fields are present", async () => {
    mockAdjacentSurfacesFetch();

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID={successfulReplaySessionID} />,
    );

    await waitFor(() => {
      expect(screen.getByText("Dispatches")).toBeTruthy();
    });

    expectLiveProviderSummaryState();

    const user = userEvent.setup();
    await user.click(
      screen.getByRole("button", {
        name: `Expand dispatch detail for ${successfulLiveProviderDispatchID}`,
      }),
    );

    await waitFor(() => {
      expect(screen.getByText("JavaScript task")).toBeTruthy();
    });
    expect(
      screen.getByRole("link", { name: "art-js-success-001" }),
    ).toBeTruthy();

    await user.click(
      screen.getByRole("button", { name: "Expand Factory Event replay" }),
    );

    await waitFor(() => {
      expect(screen.getByText("Showing 5 Factory Events.")).toBeTruthy();
    });
    expect(screen.getByText("Session started")).toBeTruthy();
    expect(
      screen.getAllByText(/artifact-release-notes/).length,
    ).toBeGreaterThan(0);
    expectLiveProviderSummaryState();
  });
});

describe("FactorySessionDetailPanel adjacent surfaces dispatch drilldown states", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("shows dispatch-detail loading while preserving live-provider summary and event replay controls", async () => {
    const dispatchDetailResponse = createDeferred<Response>();
    mockAdjacentSurfacesFetch({ deferDispatchDetail: dispatchDetailResponse });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID={successfulReplaySessionID} />,
    );

    await waitFor(() => {
      expect(screen.getByText("Dispatches")).toBeTruthy();
    });

    expectLiveProviderSummaryState();
    expect(
      screen.getByRole("button", { name: "Expand Factory Event replay" }),
    ).toBeTruthy();

    const user = userEvent.setup();
    await user.click(
      screen.getByRole("button", {
        name: `Expand dispatch detail for ${successfulLiveProviderDispatchID}`,
      }),
    );

    expect(screen.getByText("Loading dispatch detail…")).toBeTruthy();
    expectLiveProviderSummaryState();

    dispatchDetailResponse.resolve(
      jsonResponse(buildSuccessfulLiveProviderDispatchDetail()),
    );

    await waitFor(() => {
      expect(screen.getByText("Provider sessions")).toBeTruthy();
    });
    expectLiveProviderSummaryState();
  });

  it("scopes dispatch-detail missing and error states without collapsing summary or event replay", async () => {
    mockAdjacentSurfacesFetch({ dispatchDetailStatus: 404 });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID={successfulReplaySessionID} />,
    );

    await waitFor(() => {
      expect(screen.getByText("Dispatches")).toBeTruthy();
    });

    const user = userEvent.setup();
    await user.click(
      screen.getByRole("button", {
        name: `Expand dispatch detail for ${successfulLiveProviderDispatchID}`,
      }),
    );

    await waitFor(() => {
      expect(
        screen.getByText(
          `Dispatch detail for ${successfulLiveProviderDispatchID} is no longer available.`,
        ),
      ).toBeTruthy();
    });

    expectLiveProviderSummaryState();

    await user.click(
      screen.getByRole("button", { name: "Expand Factory Event replay" }),
    );

    await waitFor(() => {
      expect(screen.getByText("Showing 5 Factory Events.")).toBeTruthy();
    });
    expectLiveProviderSummaryState();
  });

  it("scopes dispatch-detail read errors without collapsing summary or event replay", async () => {
    mockAdjacentSurfacesFetch({ dispatchDetailStatus: 500 });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID={successfulReplaySessionID} />,
    );

    await waitFor(() => {
      expect(screen.getByText("Dispatches")).toBeTruthy();
    });

    const user = userEvent.setup();
    await user.click(
      screen.getByRole("button", {
        name: `Expand dispatch detail for ${successfulLiveProviderDispatchID}`,
      }),
    );

    await waitFor(() => {
      expect(screen.getByText("dispatch boom")).toBeTruthy();
    });

    expectLiveProviderSummaryState();

    await user.click(
      screen.getByRole("button", { name: "Expand Factory Event replay" }),
    );

    await waitFor(() => {
      expect(screen.getByText("Showing 5 Factory Events.")).toBeTruthy();
    });
    expectLiveProviderSummaryState();
  });
});
