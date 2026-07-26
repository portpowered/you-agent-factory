import { describe, expect, it } from "vitest";

import type { WorkstationLevelGuard } from "../../../../current-factory-definition/lib/workstation-guards";
import {
  findRemovedWorkstationGuardIndex,
  resolveStableWorkstationGuardRowKeys,
  type StableWorkstationGuardRowKeyState,
} from "./workstation-guard-row-keys";

const initialState = (
  guards: WorkstationLevelGuard[],
): StableWorkstationGuardRowKeyState => ({
  nextKey: 0,
  previousGuards: guards,
  rowKeys: [],
});

describe("findRemovedWorkstationGuardIndex", () => {
  it("returns the removed guard index", () => {
    const guards: WorkstationLevelGuard[] = [
      { maxVisits: 1, type: "VISIT_COUNT", workstation: "Plan" },
      { matchConfig: { inputKey: ".Name" }, type: "MATCHES_FIELDS" },
    ];
    expect(findRemovedWorkstationGuardIndex(guards, [guards[1]])).toBe(0);
  });
});

describe("resolveStableWorkstationGuardRowKeys", () => {
  it("keeps row keys stable when guard summary fields change", () => {
    const guards: WorkstationLevelGuard[] = [
      { matchConfig: { inputKey: ".Name" }, type: "MATCHES_FIELDS" },
    ];
    const seeded = resolveStableWorkstationGuardRowKeys(
      initialState(guards),
      guards,
    );
    const updated = resolveStableWorkstationGuardRowKeys(seeded, [
      {
        matchConfig: { inputKey: '.Tags["_last_output"]' },
        type: "MATCHES_FIELDS",
      },
    ]);
    expect(updated.rowKeys).toEqual(seeded.rowKeys);
  });

  it("drops the removed guard row key", () => {
    const guards: WorkstationLevelGuard[] = [
      { maxVisits: 1, type: "VISIT_COUNT", workstation: "Plan" },
      { matchConfig: { inputKey: ".Name" }, type: "MATCHES_FIELDS" },
    ];
    const seeded = resolveStableWorkstationGuardRowKeys(
      initialState(guards),
      guards,
    );
    const updated = resolveStableWorkstationGuardRowKeys(seeded, [guards[1]]);
    expect(updated.rowKeys).toEqual([seeded.rowKeys[1]]);
  });
});
