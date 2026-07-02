import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { buildSuccessfulDurableSession } from "../../../../testing/factory-session-event-replay-fixtures";
import {
  buildSuccessfulLiveProviderDispatchDetail,
  buildSuccessfulLiveProviderDispatchList,
  successfulLiveProviderDispatchID,
  successfulLiveProviderSessionID,
  successfulLiveProviderSessionRef,
} from "../../../../testing/factory-session-live-provider-inspection-fixtures";
import { FactorySessionDetailPanel } from "../factory-session-detail-panel";
import {
  jsonResponse,
  renderWithQueryClient,
} from "../test-support/factory-session-detail-panel.test-helpers";

const PROVIDER_SESSION_SUMMARY = `Provider session: ${successfulLiveProviderSessionRef.provider} / ${successfulLiveProviderSessionRef.kind} / ${successfulLiveProviderSessionRef.id}`;

function mockSuccessfulLiveProviderInspectionFetch() {
  vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
    const url = String(input);
    if (url.endsWith(`/factory-sessions/${successfulLiveProviderSessionID}`)) {
      return jsonResponse(
        buildSuccessfulDurableSession(successfulLiveProviderSessionID),
      );
    }
    if (
      url.endsWith(
        `/factory-sessions/${successfulLiveProviderSessionID}/dispatches`,
      )
    ) {
      return jsonResponse(buildSuccessfulLiveProviderDispatchList());
    }
    if (
      url.endsWith(
        `/factory-sessions/${successfulLiveProviderSessionID}/dispatches/${successfulLiveProviderDispatchID}`,
      )
    ) {
      return jsonResponse(buildSuccessfulLiveProviderDispatchDetail());
    }
    if (
      url.endsWith(
        `/factory-sessions/${successfulLiveProviderSessionID}/results?mode=final`,
      )
    ) {
      return new Response("not found", { status: 404 });
    }
    if (
      url.endsWith(
        `/factory-sessions/${successfulLiveProviderSessionID}/results?mode=partial`,
      )
    ) {
      return new Response("not found", { status: 404 });
    }
    return new Response("not found", { status: 404 });
  });
}

describe("FactorySessionDetailPanel live-provider inspection", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("shows live-provider execution mode and provider-session correlation from durable dispatch inspection", async () => {
    mockSuccessfulLiveProviderInspectionFetch();

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID={successfulLiveProviderSessionID} />,
    );

    await waitFor(() => {
      expect(screen.getByText("Dispatches")).toBeTruthy();
    });

    expect(screen.getByText("Execution mode: live")).toBeTruthy();
    expect(screen.getByText(PROVIDER_SESSION_SUMMARY)).toBeTruthy();

    const user = userEvent.setup();
    const drilldownTrigger = screen.getByRole("button", {
      name: `Expand dispatch detail for ${successfulLiveProviderDispatchID}`,
    });
    drilldownTrigger.focus();
    await user.keyboard("{Enter}");

    await waitFor(() => {
      expect(screen.getByText("JavaScript task")).toBeTruthy();
    });

    expect(screen.getByText("Execution mode")).toBeTruthy();
    expect(screen.getAllByText("live").length).toBeGreaterThan(0);
    expect(screen.getByText("Provider sessions")).toBeTruthy();
    expect(
      screen.getByText(
        `${successfulLiveProviderSessionRef.kind} · ${successfulLiveProviderSessionRef.id}`,
      ),
    ).toBeTruthy();
    expect(
      screen.getByRole("link", { name: "art-js-success-001" }),
    ).toBeTruthy();
  });
});
