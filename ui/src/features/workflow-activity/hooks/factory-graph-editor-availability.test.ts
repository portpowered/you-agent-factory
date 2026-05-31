import { describe, expect, it } from "vitest";

import type { CanonicalFactoryDefinition } from "../../factory-graph-editor/lib/factory-graph-draft-types";
import { findClassifierGraphEditorUnsupportedWorkstationName } from "./factory-graph-editor-availability";

const baseDefinition: CanonicalFactoryDefinition = {
  name: "Factory",
  workTypes: [
    {
      name: "story",
      states: [
        { name: "queued", type: "INITIAL" },
        { name: "done", type: "TERMINAL" },
      ],
    },
  ],
  workstations: [
    {
      inputs: [{ state: "queued", workType: "story" }],
      name: "review",
      outputs: [{ state: "done", workType: "story" }],
      type: "MODEL_WORKSTATION",
      worker: "writer",
    },
  ],
  workers: [
    {
      model: "gpt-5",
      name: "writer",
      type: "MODEL_WORKER",
    },
  ],
};

describe("findClassifierGraphEditorUnsupportedWorkstationName", () => {
  it("returns undefined when no classifier workstations are present", () => {
    expect(
      findClassifierGraphEditorUnsupportedWorkstationName(baseDefinition),
    ).toBeUndefined();
  });

  it("detects classifier workstations by type", () => {
    const factory: CanonicalFactoryDefinition = {
      ...baseDefinition,
      workstations: [
        {
          ...baseDefinition.workstations?.[0],
          classificationRoutes: [
            {
              label: "approved",
              output: { state: "done", workType: "story" },
            },
          ],
          type: "CLASSIFIER_WORKSTATION",
        },
      ],
    };

    expect(
      findClassifierGraphEditorUnsupportedWorkstationName(factory),
    ).toBe("review");
  });

  it("detects classifier workstations by labeled routes without explicit type", () => {
    const factory: CanonicalFactoryDefinition = {
      ...baseDefinition,
      workstations: [
        {
          ...baseDefinition.workstations?.[0],
          classificationRoutes: [
            {
              label: "approved",
              output: { state: "done", workType: "story" },
            },
          ],
          type: "MODEL_WORKSTATION",
        },
      ],
    };

    expect(
      findClassifierGraphEditorUnsupportedWorkstationName(factory),
    ).toBe("review");
  });
});
