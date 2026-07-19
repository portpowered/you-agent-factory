import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";
import {
  loadFactoryEmulatorRuntimeReferences,
  safeParseFactoryEmulatorRuntimeReference,
} from "./runtime-reference.js";
import { compareFactoryEmulatorRuntimeReference } from "./runtime-reference-conformance.js";
import { runtimeReferenceFixtures } from "./runtime-reference-fixtures.js";

function headingAnchor(heading: string): string {
  return heading
    .toLowerCase()
    .replaceAll(/[^\w\s-]/g, "")
    .replaceAll(/\s+/g, "-");
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: This cohesive fixture-contract suite keeps loading and rejection evidence adjacent.
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

  it("does not recompute frozen evidence when an executable fixture input changes", async () => {
    const reference = structuredClone(runtimeReferenceFixtures[0]);
    if (!Array.isArray(reference.scenario.initialSubmissions)) {
      throw new Error("Basic reference requires direct initial submissions.");
    }
    reference.scenario.initialSubmissions[0].input = "changed executable input";
    await expect(
      compareFactoryEmulatorRuntimeReference(reference),
    ).resolves.toMatchObject({
      matches: false,
      divergence: {
        fixture: "basic-execution",
        logicalTick: 3,
        surface: "routes",
        expected: ['task:done:"frozen input"'],
        actual: ['task:done:"changed executable input"'],
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
      "depends-on-release",
      "depends-on-terminal-failure",
    ]);
    for (const reference of references) {
      expect(reference.provenance.source).toContain("docs/reference/");
      expect(reference.orderedEventKinds).toEqual(
        reference.ticks.flatMap(({ eventKinds }) => eventKinds),
      );
    }
  });

  it("uses existing documentation anchors as frozen evidence provenance", () => {
    const referenceDirectory = resolve(
      import.meta.dirname,
      "../../../../docs/reference",
    );
    for (const reference of loadFactoryEmulatorRuntimeReferences()) {
      const source = reference.provenance.source.match(
        /^docs\/reference\/(.+)#([^\s]+) \(pre-existing public runtime behavior documentation\)$/,
      );
      expect(source, reference.id).not.toBeNull();
      if (source === null)
        throw new Error(`Invalid provenance for ${reference.id}.`);
      const [, file, anchor] = source;
      const markdown = readFileSync(resolve(referenceDirectory, file), "utf8");
      const anchors = Array.from(markdown.matchAll(/^#{1,6}\s+(.+)$/gm)).map(
        ([, heading]) => headingAnchor(heading ?? ""),
      );
      expect(anchors, reference.id).toContain(anchor);
    }
  });

  it("freezes the named outcome routes, propagation payload, and eligible logical move", () => {
    const byId = new Map(
      loadFactoryEmulatorRuntimeReferences().map((reference) => [
        reference.id,
        reference,
      ]),
    );
    expect(byId.get("repeaters")?.ticks[3]?.semantics).toMatchObject({
      outcomes: ["CONTINUE"],
      routes: ['task:ready:"frozen input"'],
    });
    expect(byId.get("routing")?.ticks[3]?.semantics).toMatchObject({
      outcomes: ["REJECTED"],
      routes: ['task:failed:"frozen input"'],
      terminalStates: ["task:failed"],
    });
    expect(byId.get("propagation")?.ticks[3]?.semantics.routes).toEqual([
      'task:done:"frozen input"',
    ]);
    expect(byId.get("logical-moves")?.ticks[4]).toMatchObject({
      eventKinds: ["DISPATCH_RESPONSE"],
      semantics: {
        outcomes: ["ACCEPTED"],
        routes: ['task:done:"worker output"'],
        terminalStates: ["task:done"],
      },
    });
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

  it("does not compare a reference with a stale ordered event-kind sequence", async () => {
    const invalid = structuredClone(runtimeReferenceFixtures[0]);
    invalid.orderedEventKinds[0] = "SESSION_COMPLETED";
    await expect(
      compareFactoryEmulatorRuntimeReference(invalid),
    ).resolves.toEqual({
      matches: false,
      validationIssues: [
        {
          code: "invalid_value",
          path: ["orderedEventKinds"],
          message:
            "Ordered event kinds must exactly concatenate the logical-tick event kinds.",
        },
      ],
    });
  });

  it("does not compare a reference with renumbered logical ticks", async () => {
    const invalid = structuredClone(runtimeReferenceFixtures[0]);
    for (const [index, tick] of invalid.ticks.entries()) {
      tick.logicalTick = index + 10;
    }
    await expect(
      compareFactoryEmulatorRuntimeReference(invalid),
    ).resolves.toMatchObject({
      matches: false,
      validationIssues: expect.arrayContaining([
        expect.objectContaining({
          code: "invalid_tick_order",
          path: ["ticks", 0, "logicalTick"],
          message: "Logical ticks must be contiguous and begin at zero.",
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

describe("DEPENDS_ON frozen runtime references", () => {
  it("matches prerequisite release and terminal-failure cascade semantics", async () => {
    const dependencies = loadFactoryEmulatorRuntimeReferences().filter(
      ({ id }) =>
        ["depends-on-release", "depends-on-terminal-failure"].includes(id),
    );
    for (const reference of dependencies) {
      await expect(
        compareFactoryEmulatorRuntimeReference(reference),
      ).resolves.toEqual({ matches: true });
    }
  });

  it("reports a dependency dispatch mismatch at its first divergent tick", async () => {
    const reference = structuredClone(
      runtimeReferenceFixtures.find(({ id }) => id === "depends-on-release"),
    );
    if (reference === undefined) throw new Error("Missing dependency fixture.");
    reference.ticks[2].semantics.dispatchChoices = ["execute:blocked"];
    await expect(
      compareFactoryEmulatorRuntimeReference(reference),
    ).resolves.toEqual({
      matches: false,
      divergence: {
        fixture: "depends-on-release",
        logicalTick: 2,
        surface: "dispatchChoices",
        expected: ["execute:blocked"],
        actual: ["execute:prerequisite"],
      },
    });
  });
});
