import { describe, expect, it } from "vitest";

import {
  resolveSharedWorkerWorkstationNames,
  resolveSharedWorkerWorkstationNamesByWorkerName,
} from "./workstation-editable-resolution";

describe("workstation editable resolution edge cases", () => {
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
