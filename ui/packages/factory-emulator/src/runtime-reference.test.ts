import { describe, expect, it } from "vitest";
import {
  loadFactoryEmulatorRuntimeReferences,
  safeParseFactoryEmulatorRuntimeReference,
} from "./runtime-reference.js";
import { compareFactoryEmulatorRuntimeReference } from "./runtime-reference-conformance.js";
import { runtimeReferenceFixtures } from "./runtime-reference-fixtures.js";

describe("frozen runtime references", () => {
  it("compares each supported reference at every logical tick", async () => {
    for (const reference of loadFactoryEmulatorRuntimeReferences()) {
      await expect(
        compareFactoryEmulatorRuntimeReference(reference),
      ).resolves.toEqual({
        matches: true,
      });
    }
  });

  it("reports the fixture, first divergent tick, surface, expected, and actual semantics", async () => {
    const reference = structuredClone(runtimeReferenceFixtures[0]);
    reference.ticks[2].semantics.dispatchChoices = ["wrong:input"];
    await expect(
      compareFactoryEmulatorRuntimeReference(reference),
    ).resolves.toEqual({
      matches: false,
      divergence: {
        fixture: "basic-execution",
        logicalTick: 2,
        surface: "dispatchChoices",
        expected: ["wrong:input"],
        actual: ["execute:input"],
      },
    });
  });

  it("loads documented references for every first-story behavior in canonical tick order", () => {
    const references = loadFactoryEmulatorRuntimeReferences();
    expect(references.map(({ id }) => id)).toEqual([
      "basic-execution",
      "repeaters",
      "routing",
      "logical-moves",
      "multi-input-output",
      "propagation",
      "parallel-dispatch",
      "simultaneous-completion",
      "resource-contention",
    ]);
    for (const reference of references) {
      expect(reference.provenance.source).toContain("README.md");
      expect(reference.orderedEventKinds).toEqual(
        reference.ticks.flatMap(({ eventKinds }) => eventKinds),
      );
    }
  });

  it("rejects missing reference evidence and invalid fixture schemas", () => {
    const missing = structuredClone(runtimeReferenceFixtures[0]) as Record<
      string,
      unknown
    >;
    delete missing.ticks;
    const schema = structuredClone(runtimeReferenceFixtures[0]) as Record<
      string,
      unknown
    >;
    schema.schemaVersion = "runtime-reference/v0";
    expect(safeParseFactoryEmulatorRuntimeReference(missing)).toMatchObject({
      success: false,
      issues: expect.arrayContaining([
        expect.objectContaining({
          code: "missing_required_data",
          path: ["ticks"],
        }),
      ]),
    });
    expect(safeParseFactoryEmulatorRuntimeReference(schema)).toMatchObject({
      success: false,
      issues: expect.arrayContaining([
        expect.objectContaining({ code: "invalid_schema_version" }),
      ]),
    });
  });

  it("rejects unexpected event kinds before a semantic comparison can start", () => {
    const invalid = structuredClone(runtimeReferenceFixtures[0]);
    (invalid.ticks[1]?.eventKinds as string[])[0] = "NOT_A_FACTORY_EVENT";
    expect(safeParseFactoryEmulatorRuntimeReference(invalid)).toMatchObject({
      success: false,
      issues: expect.arrayContaining([
        expect.objectContaining({
          code: "unexpected_event_kind",
          path: ["ticks", 1, "eventKinds", 0],
        }),
      ]),
    });
  });

  it("rejects a fixture that omits per-tick semantic evidence", () => {
    const invalid = structuredClone(runtimeReferenceFixtures[0]) as {
      ticks: { semantics?: unknown }[];
    };
    delete invalid.ticks[0]?.semantics;
    expect(safeParseFactoryEmulatorRuntimeReference(invalid)).toMatchObject({
      success: false,
      issues: expect.arrayContaining([
        expect.objectContaining({
          code: "missing_semantics",
          path: ["ticks", 0, "semantics"],
        }),
      ]),
    });
  });
});

describe("concurrent frozen runtime references", () => {
  it("keeps concurrent dispatch and completion references deterministic", async () => {
    const concurrent = loadFactoryEmulatorRuntimeReferences().filter(({ id }) =>
      [
        "parallel-dispatch",
        "simultaneous-completion",
        "resource-contention",
      ].includes(id),
    );
    for (const reference of concurrent) {
      await expect(
        compareFactoryEmulatorRuntimeReference(reference),
      ).resolves.toEqual({ matches: true });
    }
  });

  it("reports the first concurrent dispatch divergence", async () => {
    const reference = structuredClone(
      runtimeReferenceFixtures.find(({ id }) => id === "resource-contention"),
    );
    if (reference === undefined) throw new Error("Missing contention fixture.");
    reference.ticks[2].semantics.dispatchChoices = ["execute:third"];
    await expect(
      compareFactoryEmulatorRuntimeReference(reference),
    ).resolves.toMatchObject({
      matches: false,
      divergence: {
        fixture: "resource-contention",
        logicalTick: 2,
        surface: "dispatchChoices",
        expected: ["execute:third"],
      },
    });
  });
});
