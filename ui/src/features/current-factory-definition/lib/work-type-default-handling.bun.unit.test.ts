import { describe, expect, it } from "bun:test";

import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import { workTypeHasDefaultHandling } from "./work-type-default-handling";

describe("workTypeHasDefaultHandling", () => {
  const factory = {
    workTypes: [
      {
        handlingBehavior: ["DEFAULT"],
        name: "story",
        states: [{ name: "queued", type: "INITIAL" }],
      },
      {
        name: "task",
        states: [{ name: "queued", type: "INITIAL" }],
      },
    ],
  } satisfies CanonicalFactoryDefinition;

  it("returns true when the work type includes DEFAULT handling", () => {
    expect(workTypeHasDefaultHandling(factory, "story")).toBe(true);
  });

  it("returns false when the work type is not default", () => {
    expect(workTypeHasDefaultHandling(factory, "task")).toBe(false);
  });

  it("returns false when the factory or work type is missing", () => {
    expect(workTypeHasDefaultHandling(undefined, "story")).toBe(false);
    expect(workTypeHasDefaultHandling(factory, "missing")).toBe(false);
  });
});
