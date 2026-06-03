import { renderHook } from "@testing-library/react";

import type { WorkstationLevelGuard } from "../../../current-factory-definition/lib/workstation-guards";
import {
  findRemovedWorkstationGuardIndex,
  useStableWorkstationGuardRowKeys,
} from "./workstation-guard-row-keys";

describe("findRemovedWorkstationGuardIndex", () => {
  it("returns the removed guard index", () => {
    const previousGuards: WorkstationLevelGuard[] = [
      {
        maxVisits: 1,
        type: "VISIT_COUNT",
        workstation: "Plan",
      },
      {
        matchConfig: { inputKey: ".Name" },
        type: "MATCHES_FIELDS",
      },
    ];
    const nextGuards: WorkstationLevelGuard[] = [previousGuards[1]];

    expect(findRemovedWorkstationGuardIndex(previousGuards, nextGuards)).toBe(
      0,
    );
  });
});

describe("useStableWorkstationGuardRowKeys", () => {
  it("keeps row keys stable when guard summary fields change", () => {
    const initialGuards: WorkstationLevelGuard[] = [
      {
        matchConfig: { inputKey: ".Name" },
        type: "MATCHES_FIELDS",
      },
    ];

    const { rerender, result } = renderHook(
      ({ guards }) => useStableWorkstationGuardRowKeys(guards),
      { initialProps: { guards: initialGuards } },
    );

    const initialKeys = [...result.current];

    rerender({
      guards: [
        {
          matchConfig: { inputKey: '.Tags["_last_output"]' },
          type: "MATCHES_FIELDS",
        },
      ],
    });

    expect(result.current).toEqual(initialKeys);
  });

  it("drops the removed guard row key", () => {
    const guards: WorkstationLevelGuard[] = [
      {
        maxVisits: 1,
        type: "VISIT_COUNT",
        workstation: "Plan",
      },
      {
        matchConfig: { inputKey: ".Name" },
        type: "MATCHES_FIELDS",
      },
    ];

    const { rerender, result } = renderHook(
      ({ guards: currentGuards }) =>
        useStableWorkstationGuardRowKeys(currentGuards),
      { initialProps: { guards } },
    );

    const keysBeforeRemoval = [...result.current];

    rerender({ guards: [guards[1]] });

    expect(result.current).toEqual([keysBeforeRemoval[1]]);
  });
});
