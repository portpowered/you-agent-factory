import { describe, expect, it } from "vitest";

import {
  resolveCanonicalWorkstation,
  resolveSharedWorkerWorkstationNames,
  resolveSharedWorkerWorkstationNamesByWorkerName,
  resolveWorkerModelProvider,
  resolveWorkerOptions,
} from "./workstation-editable-resolution";

describe("workstation editable resolution edge cases", () => {
  it("resolves canonical workstations by transition id or workstation name", () => {
    const factory = {
      name: "Factory",
      workers: [{ name: "writer", model: "gpt-5", type: "MODEL_WORKER" as const }],
      workstations: [
        { id: "review-id", name: "Review", worker: "writer" },
        { name: "Plan", worker: "writer" },
      ],
    };

    expect(
      resolveCanonicalWorkstation(factory, {
        node_id: "review",
        transition_id: "review-id",
        workstation_kind: "MODEL_WORKSTATION",
        workstation_name: "Review",
      }),
    ).toEqual({
      workstation: factory.workstations?.[0],
      workstationIndex: 0,
    });

    expect(
      resolveCanonicalWorkstation(factory, {
        node_id: "plan",
        transition_id: "missing",
        workstation_kind: "MODEL_WORKSTATION",
        workstation_name: "Plan",
      }),
    ).toEqual({
      workstation: factory.workstations?.[1],
      workstationIndex: 1,
    });
  });

  it("exposes worker catalog helpers for editable workstation forms", () => {
    const factory = {
      name: "Factory",
      workers: [
        {
          name: "writer",
          model: "gpt-5",
          modelProvider: "CURSOR",
          type: "MODEL_WORKER" as const,
        },
      ],
      workstations: [],
    };

    expect(resolveWorkerOptions(factory)).toEqual(["writer"]);
    expect(resolveWorkerModelProvider(factory, "writer")).toBe("CURSOR");
    expect(resolveWorkerModelProvider(factory, "missing")).toBeNull();
  });

  it("returns no shared worker workstations when the selected workstation has no worker", () => {
    expect(
      resolveSharedWorkerWorkstationNames(
        {
          name: "Factory",
          workstations: [
            { name: "solo" },
            { name: "shared", worker: "writer" },
          ],
        },
        { name: "solo" },
        0,
      ),
    ).toEqual([]);
  });

  it("skips workstations without workers when building shared worker maps", () => {
    expect(
      resolveSharedWorkerWorkstationNamesByWorkerName(
        {
          name: "Factory",
          workstations: [
            { id: "selected", name: "selected" },
            { name: "orphan" },
            { name: "shared", worker: "writer" },
            { name: "", worker: "writer" },
          ],
        },
        { id: "selected", name: "selected" },
      ),
    ).toEqual({
      writer: ["shared"],
    });
  });
});
