import { describe, expect, it } from "vitest";
import {
  factoryGraphConnectionAnchorContext,
  getLocalizedFactoryGraphConnectionAnchors,
  isValidFactoryGraphConnection,
} from "../factory-graph-editor-connections";

const humanApprovalContext = factoryGraphConnectionAnchorContext({
  behavior: "STANDARD",
  type: "HUMAN_APPROVAL",
});

describe("human approval graph editor connections", () => {
  it("exposes workerless approval and rejection handles", () => {
    const anchorIds = getLocalizedFactoryGraphConnectionAnchors(
      "workstation",
      "en",
      humanApprovalContext,
    ).map((anchor) => anchor.id);

    expect(anchorIds).toEqual(
      expect.arrayContaining([
        "workstation-input-target",
        "workstation-approval-source",
        "workstation-on-rejection-source",
      ]),
    );
    expect(anchorIds).not.toContain("worker-assignment-target");
    expect(anchorIds).not.toContain("workstation-resource-target");
    expect(anchorIds).not.toContain("workstation-output-source");
    expect(anchorIds).not.toContain("workstation-on-continue-source");
    expect(anchorIds).not.toContain("workstation-on-failure-source");
  });

  it("only accepts approval and rejection route connections", () => {
    expect(
      isValidFactoryGraphConnection({
        sourceAnchorId: "workstation-approval-source",
        sourceNodeKind: "workstation",
        targetAnchorId: "work-state-input-target",
        targetNodeKind: "work-state",
        sourceWorkstation: humanApprovalContext.workstation,
      }),
    ).toBe(true);
    expect(
      isValidFactoryGraphConnection({
        sourceAnchorId: "workstation-on-continue-source",
        sourceNodeKind: "workstation",
        targetAnchorId: "work-state-input-target",
        targetNodeKind: "work-state",
        sourceWorkstation: humanApprovalContext.workstation,
      }),
    ).toBe(false);
  });
});
