import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  buildFailedBridgedChildDispatchDetail,
  buildFailedBridgedChildDispatchList,
  buildFailedBridgedChildDurableSession,
  failedBridgedChildDispatchID,
  failedBridgedChildProviderSessionRef,
  failedBridgedChildSessionID,
} from "../../../../testing/factory-session-live-provider-inspection-fixtures";
import { FactorySessionDetailPanel } from "../factory-session-detail-panel";
import {
  jsonResponse,
  renderWithQueryClient,
} from "../test-support/factory-session-detail-panel.test-helpers";

const PROVIDER_SESSION_SUMMARY = `Provider session: ${failedBridgedChildProviderSessionRef.provider} / ${failedBridgedChildProviderSessionRef.kind} / ${failedBridgedChildProviderSessionRef.id}`;

function mockFailedBridgedChildInspectionFetch(
  dispatchDetail = buildFailedBridgedChildDispatchDetail(),
) {
  vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
    const url = String(input);
    if (url.endsWith(`/factory-sessions/${failedBridgedChildSessionID}`)) {
      return jsonResponse(
        buildFailedBridgedChildDurableSession(failedBridgedChildSessionID),
      );
    }
    if (
      url.endsWith(
        `/factory-sessions/${failedBridgedChildSessionID}/dispatches`,
      )
    ) {
      return jsonResponse(buildFailedBridgedChildDispatchList());
    }
    if (
      url.endsWith(
        `/factory-sessions/${failedBridgedChildSessionID}/dispatches/${failedBridgedChildDispatchID}`,
      )
    ) {
      return jsonResponse(dispatchDetail);
    }
    if (
      url.endsWith(
        `/factory-sessions/${failedBridgedChildSessionID}/results?mode=final`,
      )
    ) {
      return new Response("not found", { status: 404 });
    }
    if (
      url.endsWith(
        `/factory-sessions/${failedBridgedChildSessionID}/results?mode=partial`,
      )
    ) {
      return new Response("not found", { status: 404 });
    }
    return new Response("not found", { status: 404 });
  });
}

describe("FactorySessionDetailPanel failed bridged-child inspection", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("shows typed failure detail with execution mode and provider-session correlation from durable dispatch inspection", async () => {
    mockFailedBridgedChildInspectionFetch();

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID={failedBridgedChildSessionID} />,
    );

    await waitFor(() => {
      expect(screen.getByText("Dispatches")).toBeTruthy();
    });

    expect(screen.getByText("Execution mode: live")).toBeTruthy();
    expect(screen.getByText(PROVIDER_SESSION_SUMMARY)).toBeTruthy();

    const user = userEvent.setup();
    const drilldownTrigger = screen.getByRole("button", {
      name: `Expand dispatch detail for ${failedBridgedChildDispatchID}`,
    });
    drilldownTrigger.focus();
    await user.keyboard("{Enter}");

    await waitFor(() => {
      expect(screen.getByText("Failure detail")).toBeTruthy();
    });

    expect(screen.getByText("provider_version_incompatible")).toBeTruthy();
    expect(screen.getByText("provider_error")).toBeTruthy();
    expect(
      screen.getByText(
        "Model gpt-5.6-sol requires a newer Codex version. Upgrade Codex and retry.",
      ),
    ).toBeTruthy();
    expect(screen.getAllByText("live").length).toBeGreaterThan(0);
    expect(screen.getByText("Provider sessions")).toBeTruthy();
    expect(
      screen.getByText(
        `${failedBridgedChildProviderSessionRef.kind} · ${failedBridgedChildProviderSessionRef.id}`,
      ),
    ).toBeTruthy();
  });

  it("omits failure detail when the durable dispatch read has no typed failure payload", async () => {
    const { failureDetail: _failureDetail, ...dispatchWithoutFailureDetail } =
      buildFailedBridgedChildDispatchDetail();
    mockFailedBridgedChildInspectionFetch(dispatchWithoutFailureDetail);

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID={failedBridgedChildSessionID} />,
    );

    await waitFor(() => {
      expect(screen.getByText("Dispatches")).toBeTruthy();
    });

    const user = userEvent.setup();
    await user.click(
      screen.getByRole("button", {
        name: `Expand dispatch detail for ${failedBridgedChildDispatchID}`,
      }),
    );

    await waitFor(() => {
      expect(screen.getByText("JavaScript task")).toBeTruthy();
    });

    expect(screen.queryByText("Failure detail")).toBeNull();
    expect(screen.getByText("live")).toBeTruthy();
    expect(screen.getByText("codex")).toBeTruthy();
  });
});
