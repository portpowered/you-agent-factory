import { describe, expect, it } from "vitest";
import {
  FACTORY_EMULATOR_SCHEDULER_CANDIDATE_LIMIT,
  type FactorySchedulerCandidate,
  selectFactorySchedulerCandidates,
} from "./scheduler.js";

interface CandidateOptions {
  readonly tokenId?: string;
  readonly customerWork?: boolean;
  readonly processing?: boolean;
  readonly queuedElapsedMs?: number;
  readonly lastDispatchElapsedMs?: number;
  readonly transitionId?: string;
  readonly workerId?: string;
  readonly workstationKind?: "logical" | "normal";
  readonly resources?: readonly {
    readonly name: string;
    readonly capacity: number;
  }[];
}

function candidate(
  value: string,
  options: CandidateOptions = {},
): FactorySchedulerCandidate<string> {
  return {
    transitionId: options.transitionId ?? value,
    workerId: options.workerId ?? "worker",
    workstationKind: options.workstationKind ?? "normal",
    resources: options.resources ?? [],
    tokens: [
      {
        tokenId: options.tokenId ?? `token-${value}`,
        customerWork: options.customerWork ?? true,
        processing: options.processing ?? false,
        queuedElapsedMs: options.queuedElapsedMs ?? 0,
        ...(options.lastDispatchElapsedMs === undefined
          ? {}
          : { lastDispatchElapsedMs: options.lastDispatchElapsedMs }),
      },
    ],
    value,
  };
}

function selectedValues(
  candidates: readonly FactorySchedulerCandidate<string>[],
): readonly string[] {
  return selectFactorySchedulerCandidates(candidates).map(({ value }) => value);
}

describe("Factory emulator Work-in-queue scheduler", () => {
  it("applies every precedence dimension before stable identifiers", () => {
    const winnerFor = (
      left: FactorySchedulerCandidate<string>,
      right: FactorySchedulerCandidate<string>,
    ) => selectedValues([right, left])[0];

    expect(
      winnerFor(
        candidate("processing", { processing: true }),
        candidate("initial"),
      ),
    ).toBe("processing");
    expect(
      winnerFor(
        candidate("customer"),
        candidate("internal", { customerWork: false }),
      ),
    ).toBe("customer");
    expect(
      winnerFor(
        candidate("logical", { workstationKind: "logical" }),
        candidate("worker"),
      ),
    ).toBe("logical");
    expect(
      winnerFor(
        candidate("initialized", { lastDispatchElapsedMs: 20 }),
        candidate("uninitialized", { queuedElapsedMs: 0 }),
      ),
    ).toBe("initialized");
    expect(
      winnerFor(
        candidate("older-dispatch", { lastDispatchElapsedMs: 10 }),
        candidate("newer-dispatch", { lastDispatchElapsedMs: 20 }),
      ),
    ).toBe("older-dispatch");
    expect(
      winnerFor(
        candidate("older-queue", { queuedElapsedMs: 10 }),
        candidate("newer-queue", { queuedElapsedMs: 20 }),
      ),
    ).toBe("older-queue");
  });

  it("uses transition, worker, and token identifiers as deterministic ties", () => {
    const tied = [
      candidate("token-z", {
        transitionId: "transition-b",
        workerId: "worker-a",
        tokenId: "token-z",
      }),
      candidate("worker-b", {
        transitionId: "transition-a",
        workerId: "worker-b",
      }),
      candidate("token-b", {
        transitionId: "transition-b",
        workerId: "worker-a",
        tokenId: "token-b",
      }),
      candidate("worker-a", {
        transitionId: "transition-a",
        workerId: "worker-a",
      }),
    ];

    expect(selectedValues(tied)).toEqual([
      "worker-a",
      "worker-b",
      "token-b",
      "token-z",
    ]);
    expect(selectedValues([...tied].reverse())).toEqual(selectedValues(tied));
  });

  it("claims higher-ranked tokens and still selects independent Work", () => {
    const selected = selectFactorySchedulerCandidates([
      candidate("shared-lower", {
        tokenId: "shared",
        transitionId: "z-lower",
      }),
      candidate("independent", { tokenId: "independent" }),
      candidate("shared-winner", {
        tokenId: "shared",
        transitionId: "a-winner",
      }),
    ]);

    expect(selected.map(({ value }) => value)).toEqual([
      "shared-winner",
      "independent",
    ]);
  });

  it("selects at most 50 candidates reproducibly", () => {
    const candidates = Array.from({ length: 75 }, (_, index) =>
      candidate(`candidate-${String(index).padStart(2, "0")}`),
    ).reverse();
    const first = selectedValues(candidates);
    const second = selectedValues([...candidates].reverse());

    expect(first).toHaveLength(FACTORY_EMULATOR_SCHEDULER_CANDIDATE_LIMIT);
    expect(second).toEqual(first);
    expect(first.at(-1)).toBe("candidate-49");
  });
});

describe("Factory emulator resource claims", () => {
  it("claims complete requirements atomically in ranked order", () => {
    const selected = selectFactorySchedulerCandidates(
      [
        candidate("blocked-winner", {
          transitionId: "a-blocked",
          resources: [
            { name: "gpu", capacity: 1 },
            { name: "slot", capacity: 2 },
          ],
        }),
        candidate("gpu-fallback", {
          transitionId: "b-fallback",
          resources: [{ name: "gpu", capacity: 1 }],
        }),
        candidate("slot-fallback", {
          transitionId: "c-fallback",
          resources: [{ name: "slot", capacity: 1 }],
        }),
      ],
      undefined,
      { gpu: 1, slot: 1 },
    );

    expect(selected.map(({ value }) => value)).toEqual([
      "gpu-fallback",
      "slot-fallback",
    ]);
  });

  it("never allocates aggregate capacity twice in one batch", () => {
    const selected = selectFactorySchedulerCandidates(
      ["third", "first", "second"].map((value) =>
        candidate(value, {
          resources: [{ name: "agent-slot", capacity: 1 }],
        }),
      ),
      undefined,
      { "agent-slot": 2 },
    );

    expect(selected.map(({ value }) => value)).toEqual(["first", "second"]);
  });
});
