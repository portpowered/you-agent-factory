import { describe, expect, it } from "vitest";

import {
  factoryGraphWorkerIconKind,
  isFactoryGraphKnownWorkerType,
} from "./worker-icon.js";

describe("Factory graph worker compatibility presentation", () => {
  it("uses exact canonical membership for worker kinds", () => {
    expect(isFactoryGraphKnownWorkerType("SCRIPT_WORKER")).toBe(true);
    expect(
      ["script_worker", "SCRIPT_WORKER ", " INFERENCE_WORKER"].map(
        isFactoryGraphKnownWorkerType,
      ),
    ).toEqual([false, false, false]);
  });

  it("does not select a known glyph for unfamiliar worker kinds", () => {
    expect(factoryGraphWorkerIconKind("SCRIPT_WORKER", "CODEX")).toBe("script");
    expect(factoryGraphWorkerIconKind("script_worker", "CODEX")).toBe("worker");
    expect(factoryGraphWorkerIconKind("SCRIPT_WORKER ", "CODEX")).toBe(
      "worker",
    );
    expect(factoryGraphWorkerIconKind("FUTURE_WORKER_KIND", "CLAUDE")).toBe(
      "worker",
    );
    expect(factoryGraphWorkerIconKind(undefined, "CODEX")).toBe("codex");
  });
});
