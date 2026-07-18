import assert from "node:assert/strict";
import test from "node:test";
import { deriveFactoryEmulatorIdentity } from "../src/identity.js";

test("every runtime identity kind is stable, domain-separated, and seed-sensitive", () => {
  const kinds = [
    "session", "request", "trace", "work", "token", "dispatch", "completion", "event",
  ];
  const stableCoordinates = {
    definition: { workTypes: ["checkout"], name: "factory" },
    scenarioId: "checkout",
    seed: "seed-a",
    lineage: ["root"],
    logicalSequence: 3,
    command: { kind: "start" },
  };
  const reorderedCoordinates = {
    command: { kind: "start" },
    logicalSequence: 3,
    lineage: ["root"],
    seed: "seed-a",
    scenarioId: "checkout",
    definition: { name: "factory", workTypes: ["checkout"] },
  };

  const first = kinds.map((kind) => deriveFactoryEmulatorIdentity(kind, stableCoordinates));
  const repeated = kinds.map(
    (kind) => deriveFactoryEmulatorIdentity(kind, reorderedCoordinates),
  );
  const changedSeed = kinds.map((kind) => deriveFactoryEmulatorIdentity(kind, {
    ...stableCoordinates,
    seed: "seed-b",
  }));

  assert.deepEqual(repeated, first);
  assert.equal(new Set(first).size, kinds.length);
  assert.ok(changedSeed.every((identity, index) => identity !== first[index]));
});
