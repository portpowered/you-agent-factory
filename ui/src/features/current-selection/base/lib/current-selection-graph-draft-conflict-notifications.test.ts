import { describe, expect, it } from "vitest";

import { getCurrentSelectionGraphDraftConflictMessages } from "../messages/operational/current-selection-graph-draft-conflict";
import {
  CURRENT_SELECTION_GRAPH_DRAFT_CONFLICT_WARNING_KEY,
  resolveCurrentSelectionGraphDraftConflictNotification,
} from "./current-selection-graph-draft-conflict-notifications";

const conflictInputs = {
  graphDraftHasPendingChanges: true,
  isTopologyAffectingSave: true,
  saveSucceeded: true,
} as const;

describe("resolveCurrentSelectionGraphDraftConflictNotification", () => {
  it("returns a warning payload when save succeeded, topology changed, and graph draft is dirty", () => {
    const messages = getCurrentSelectionGraphDraftConflictMessages("en");

    expect(
      resolveCurrentSelectionGraphDraftConflictNotification({
        ...conflictInputs,
        locale: "en",
      }),
    ).toEqual({
      description: messages.graphDraftConflictWarningDescription,
      key: CURRENT_SELECTION_GRAPH_DRAFT_CONFLICT_WARNING_KEY,
      kind: "warning",
      title: messages.graphDraftConflictWarningTitle,
    });
  });

  it("returns null when the graph draft is clean", () => {
    expect(
      resolveCurrentSelectionGraphDraftConflictNotification({
        ...conflictInputs,
        graphDraftHasPendingChanges: false,
        locale: "en",
      }),
    ).toBeNull();
  });

  it("returns null when the save was not topology-affecting", () => {
    expect(
      resolveCurrentSelectionGraphDraftConflictNotification({
        ...conflictInputs,
        isTopologyAffectingSave: false,
        locale: "en",
      }),
    ).toBeNull();
  });

  it("returns null when the save did not succeed", () => {
    expect(
      resolveCurrentSelectionGraphDraftConflictNotification({
        ...conflictInputs,
        locale: "en",
        saveSucceeded: false,
      }),
    ).toBeNull();
  });

  it("resolves localized title and description for the requested locale", () => {
    const messages = getCurrentSelectionGraphDraftConflictMessages("ja");

    expect(
      resolveCurrentSelectionGraphDraftConflictNotification({
        ...conflictInputs,
        locale: "ja",
      }),
    ).toEqual({
      description: messages.graphDraftConflictWarningDescription,
      key: CURRENT_SELECTION_GRAPH_DRAFT_CONFLICT_WARNING_KEY,
      kind: "warning",
      title: messages.graphDraftConflictWarningTitle,
    });
  });
});
