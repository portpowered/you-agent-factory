import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  buildEmptyReplayEventStream,
  buildSuccessfulDurableSession,
  buildWarningDurableSession,
  successfulReplaySessionID,
  warningReplaySessionID,
} from "../../../../testing/factory-session-event-replay-fixtures";
import { FactorySessionDetailPanel } from "../factory-session-detail-panel";
import {
  createDeferred,
  jsonResponse,
  renderWithQueryClient,
} from "../test-support/factory-session-detail-panel.test-helpers";

describe("FactorySessionDetailPanel event replay disclosure loading and empty states", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("renders an explicit loading state while durable replay is being read", async () => {
    const replayResponse = createDeferred<Response>();

    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const url = String(input);
      if (url.endsWith(`/factory-sessions/${successfulReplaySessionID}`)) {
        return jsonResponse(buildSuccessfulDurableSession());
      }
      if (
        url.endsWith(`/factory-sessions/${successfulReplaySessionID}/events`)
      ) {
        return replayResponse.promise;
      }
      return new Response("not found", { status: 404 });
    });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID={successfulReplaySessionID} />,
    );

    await waitFor(() => {
      expect(screen.getByText("JavaScript workflow")).toBeTruthy();
    });

    const user = userEvent.setup();
    await user.click(
      screen.getByRole("button", { name: "Expand Factory Event replay" }),
    );

    await waitFor(() => {
      expect(
        screen.getByText("Loading durable Factory Event replay…"),
      ).toBeTruthy();
    });

    replayResponse.resolve(
      new Response("", {
        headers: {
          "Content-Type": "text/event-stream",
        },
        status: 200,
      }),
    );

    await waitFor(() => {
      expect(
        screen.getByText(
          "No durable Factory Events are available for this session.",
        ),
      ).toBeTruthy();
    });
  });

  it("renders an explicit empty state when durable replay has no events in scope", async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const url = String(input);
      if (url.endsWith(`/factory-sessions/${successfulReplaySessionID}`)) {
        return jsonResponse(buildSuccessfulDurableSession());
      }
      if (
        url.endsWith(`/factory-sessions/${successfulReplaySessionID}/events`)
      ) {
        return new Response(buildEmptyReplayEventStream(), {
          headers: {
            "Content-Type": "text/event-stream",
          },
          status: 200,
        });
      }
      return new Response("not found", { status: 404 });
    });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID={successfulReplaySessionID} />,
    );

    await waitFor(() => {
      expect(screen.getByText("JavaScript workflow")).toBeTruthy();
    });

    const user = userEvent.setup();
    await user.click(
      screen.getByRole("button", { name: "Expand Factory Event replay" }),
    );

    await waitFor(() => {
      expect(
        screen.getByText(
          "No durable Factory Events are available for this session.",
        ),
      ).toBeTruthy();
    });
  });
});

describe("FactorySessionDetailPanel event replay disclosure unavailable and error states", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("renders an explicit unavailable state when durable replay is omitted for the session", async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const url = String(input);
      if (url.endsWith(`/factory-sessions/${warningReplaySessionID}`)) {
        return jsonResponse(buildWarningDurableSession());
      }
      if (url.endsWith(`/factory-sessions/${warningReplaySessionID}/events`)) {
        return new Response("not found", { status: 404 });
      }
      return new Response("not found", { status: 404 });
    });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID={warningReplaySessionID} />,
    );

    await waitFor(() => {
      expect(screen.getByText("JavaScript workflow")).toBeTruthy();
    });

    const user = userEvent.setup();
    await user.click(
      screen.getByRole("button", { name: "Expand Factory Event replay" }),
    );

    await waitFor(() => {
      expect(
        screen.getByText(
          "Durable Factory Event replay is unavailable for this session.",
        ),
      ).toBeTruthy();
    });

    expect(screen.getByText("Partial result ref")).toBeTruthy();
  });

  it("renders an explicit error state when the durable replay read fails", async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const url = String(input);
      if (url.endsWith(`/factory-sessions/${warningReplaySessionID}`)) {
        return jsonResponse(buildWarningDurableSession());
      }
      if (url.endsWith(`/factory-sessions/${warningReplaySessionID}/events`)) {
        return new Response(
          JSON.stringify({
            code: "INTERNAL_ERROR",
            message: "replay boom",
          }),
          {
            headers: { "Content-Type": "application/json" },
            status: 500,
          },
        );
      }
      return new Response("not found", { status: 404 });
    });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID={warningReplaySessionID} />,
    );

    await waitFor(() => {
      expect(screen.getByText("JavaScript workflow")).toBeTruthy();
    });

    const user = userEvent.setup();
    await user.click(
      screen.getByRole("button", { name: "Expand Factory Event replay" }),
    );

    await waitFor(() => {
      expect(screen.getByText("replay boom")).toBeTruthy();
    });

    expect(screen.getByText("Artifacts")).toBeTruthy();
  });
});
