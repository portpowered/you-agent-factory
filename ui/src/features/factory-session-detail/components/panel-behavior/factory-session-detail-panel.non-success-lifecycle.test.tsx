import { screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { FactorySessionDetailPanel } from "../factory-session-detail-panel";
import { BASELINE_SESSION_ID } from "../test-support/factory-session-detail-panel.baseline-fixtures";
import {
  mockPausedLifecycleFetch,
  mockPetriRuntimeDetailFetch,
  mockRunningLifecycleFetch,
  mockSessionApiErrorFetch,
  mockSessionNotFoundFetch,
  NOT_FOUND_SESSION_ID,
  PETRI_SESSION_ID,
} from "../test-support/factory-session-detail-panel.non-success-lifecycle-fixtures";
import { renderWithQueryClient } from "../test-support/factory-session-detail-panel.test-helpers";

describe("FactorySessionDetailPanel non-success session and lifecycle states", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("shows a not-found state when the factory session is no longer available", async () => {
    mockSessionNotFoundFetch();

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID={NOT_FOUND_SESSION_ID} />,
    );

    await waitFor(() => {
      expect(
        screen.getByText("This factory session is no longer available."),
      ).toBeTruthy();
    });

    expect(screen.queryByText("Loading factory session runtime…")).toBeNull();
    expect(screen.queryByText("Factory session missing.")).toBeNull();
  });

  it("shows an error state when the factory session API fails", async () => {
    mockSessionApiErrorFetch();

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID={BASELINE_SESSION_ID} />,
    );

    await waitFor(() => {
      expect(screen.getByRole("alert").textContent).toContain("boom");
    });
    expect(screen.getByText(BASELINE_SESSION_ID)).toBeTruthy();
  });

  it("shows canonical paused Factory Session lifecycle status from the API read model", async () => {
    mockPausedLifecycleFetch();

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID={BASELINE_SESSION_ID} />,
    );

    await waitFor(() => {
      expect(screen.getByText("PAUSED")).toBeTruthy();
    });

    expect(screen.getByText("Factory Session lifecycle")).toBeTruthy();
  });

  it("shows running Factory Session lifecycle status after a canonical resume read", async () => {
    mockRunningLifecycleFetch();

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID={BASELINE_SESSION_ID} />,
    );

    await waitFor(() => {
      expect(screen.getByText("RUNNING")).toBeTruthy();
    });

    expect(screen.getByText("Factory Session lifecycle")).toBeTruthy();
  });

  it("shows Petri marking and enabled transitions without dynamic workflow shorthand", async () => {
    mockPetriRuntimeDetailFetch();

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID={PETRI_SESSION_ID} />,
    );

    await waitFor(() => {
      expect(screen.getByText("1 token")).toBeTruthy();
    });

    expect(screen.getByText("tr-process (worker-a)")).toBeTruthy();
    expect(
      screen.queryByText("Dynamic workflow (JavaScript factory session)"),
    ).toBeNull();
  });
});
