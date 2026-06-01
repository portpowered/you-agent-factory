import { describe, expect, it } from "vitest";

import * as workerSelectionPublic from "./index";

describe("worker-selection/public", () => {
  it("keeps the public runtime surface focused on WorkerDetailCard", () => {
    expect(Object.keys(workerSelectionPublic).sort()).toEqual([
      "EditableWorkerConfigurationHeaderActions",
      "EditableWorkerSaveHeaderAction",
      "WorkerDetailCard",
    ]);
    expect(workerSelectionPublic.WorkerDetailCard).toBeTypeOf("function");
  });
});
