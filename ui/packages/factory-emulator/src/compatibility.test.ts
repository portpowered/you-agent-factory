import type { FactoryDefinition } from "@you-agent-factory/client";
import { describe, expect, it, vi } from "vitest";

import {
  inspectFactoryEmulatorCompatibility,
  writeFactoryEventsIfCompatible,
} from "./compatibility.js";

const supportedFactory = {
  name: "supported-factory",
  orchestrator: { kind: "PETRI" },
  resources: [{ name: "agent-slot", capacity: 2 }],
  workTypes: [
    {
      name: "story",
      states: [
        { name: "ready", type: "INITIAL" },
        { name: "review", type: "PROCESSING" },
        { name: "done", type: "TERMINAL" },
        { name: "failed", type: "FAILED" },
      ],
    },
  ],
  workers: [
    {
      name: "executor",
      type: "AGENT_WORKER",
      resources: [{ name: "worker-metadata-only", capacity: 1 }],
    },
  ],
  workstations: [
    {
      name: "execute",
      type: "AGENT_RUN",
      behavior: "REPEATER",
      worker: "executor",
      inputs: [{ workType: "story", state: "ready" }],
      outputs: [{ workType: "story", state: "review" }],
      onContinue: [{ workType: "story", state: "ready" }],
      onRejection: [{ workType: "story", state: "ready" }],
      onFailure: [{ workType: "story", state: "failed" }],
      resources: [{ name: "agent-slot", capacity: 1 }],
      workPropagation: { mode: "PRESERVE_INPUT" },
    },
    {
      name: "loop-breaker",
      type: "LOGICAL_MOVE",
      behavior: "STANDARD",
      worker: "",
      inputs: [{ workType: "story", state: "ready" }],
      outputs: [{ workType: "story", state: "failed" }],
      guards: [
        { type: "VISIT_COUNT", workstation: "execute", maxVisits: 3 },
      ],
      workPropagation: { mode: "OUTPUT_AS_PAYLOAD" },
    },
  ],
} satisfies FactoryDefinition;

function factoryWith(
  change: (factory: FactoryDefinition) => void,
): FactoryDefinition {
  const factory = structuredClone(supportedFactory) as FactoryDefinition;
  change(factory);
  return factory;
}

function expectCodes(factory: FactoryDefinition, ...codes: string[]): void {
  const result = inspectFactoryEmulatorCompatibility(factory);
  expect(result.supported).toBe(false);
  if (result.supported) return;
  expect(result.diagnostics.map((diagnostic) => diagnostic.code)).toEqual(
    expect.arrayContaining(codes),
  );
}

describe("Factory emulator compatibility", () => {
  it("supports the planned Petri subset and omitted-orchestrator default", () => {
    expect(inspectFactoryEmulatorCompatibility(supportedFactory)).toEqual({
      supported: true,
      diagnostics: [],
    });
    const omitted = factoryWith((factory) => {
      delete factory.orchestrator;
    });
    expect(inspectFactoryEmulatorCompatibility(omitted).supported).toBe(true);
  });

  it("rejects JavaScript orchestration", () => {
    expectCodes(
      factoryWith((factory) => {
        factory.orchestrator = { kind: "JAVASCRIPT", javascript: {} };
      }),
      "unsupported_orchestrator",
    );
  });

  it.each([
    ["CRON", "AGENT_RUN"],
    ["POLLER", "POLLER_RUN"],
    ["STANDARD", "INFERENCE_RUN"],
    ["STANDARD", "SCRIPT_RUN"],
  ] as const)(
    "rejects %s behavior with %s execution",
    (behavior, type) => {
      expectCodes(
        factoryWith((factory) => {
          const workstation = factory.workstations?.[0];
          if (workstation === undefined) return;
          workstation.behavior = behavior;
          workstation.type = type;
        }),
        behavior === "STANDARD"
          ? "unsupported_execution"
          : "unsupported_workstation_behavior",
      );
    },
  );

  it("rejects classifiers, unsupported guards, and relationship-aware behavior", () => {
    const factory = factoryWith((candidate) => {
      const workstation = candidate.workstations?.[0];
      if (workstation === undefined) return;
      workstation.type = "CLASSIFIER_WORKSTATION";
      workstation.classificationRoutes = [
        {
          label: "done",
          outputs: [{ workType: "story", state: "done" }],
        },
      ];
      workstation.guards = [
        { type: "MATCHES_FIELDS", matchConfig: { inputKey: ".Name" } },
      ];
      if (workstation.inputs[0] !== undefined) {
        workstation.inputs[0].guards = [
          {
            type: "ALL_CHILDREN_COMPLETE",
            parentInput: "parent",
            spawnedBy: "execute",
          },
        ];
      }
    });
    expectCodes(
      factory,
      "unsupported_classifier",
      "unsupported_guard",
      "unsupported_relationship_behavior",
    );
  });

  it("rejects provider-global capacity and unresolved workstation requirements", () => {
    const factory = factoryWith((candidate) => {
      candidate.resources?.push({
        name: "provider-quota",
        type: "PROVIDER_QUOTA",
        provider: "CODEX",
        capacity: 1,
      });
      candidate.workstations?.[0]?.resources?.push({
        name: "missing",
        capacity: 1,
      });
    });
    expectCodes(
      factory,
      "provider_global_resource",
      "unresolved_resource_requirement",
    );
  });

  it("rejects invalid Work routes, worker references, and logical moves", () => {
    const factory = factoryWith((candidate) => {
      const agentRun = candidate.workstations?.[0];
      if (agentRun !== undefined) {
        agentRun.worker = "missing";
        agentRun.outputs = [{ workType: "story", state: "missing" }];
      }
      const logicalMove = candidate.workstations?.[1];
      if (logicalMove !== undefined) {
        logicalMove.worker = "executor";
        logicalMove.guards = [];
      }
    });
    expectCodes(
      factory,
      "invalid_worker_reference",
      "invalid_route_reference",
      "invalid_logical_move",
    );
  });

  it("returns every diagnostic in deterministic, location-aware order", () => {
    const factory = factoryWith((candidate) => {
      candidate.orchestrator = { kind: "JAVASCRIPT", javascript: {} };
      candidate.guards = [
        {
          type: "INFERENCE_THROTTLE_GUARD",
          modelProvider: "CODEX",
          refreshWindow: "1m",
        },
      ];
      const workstation = candidate.workstations?.[0];
      if (workstation !== undefined) {
        workstation.behavior = "CRON";
        workstation.type = "INFERENCE_RUN";
      }
    });
    const first = inspectFactoryEmulatorCompatibility(factory);
    const second = inspectFactoryEmulatorCompatibility(factory);
    expect(second).toEqual(first);
    expect(first).toMatchObject({
      supported: false,
      diagnostics: [
        { code: "unsupported_orchestrator", path: ["orchestrator", "kind"] },
        { code: "unsupported_guard", path: ["guards", 0] },
        {
          code: "unsupported_workstation_behavior",
          path: ["workstations", 0, "behavior"],
        },
        {
          code: "unsupported_execution",
          path: ["workstations", 0, "type"],
        },
      ],
    });
  });

  it("is pure and treats worker-only resource metadata as inspection-only", () => {
    const factory = structuredClone(supportedFactory);
    const before = structuredClone(factory);
    expect(inspectFactoryEmulatorCompatibility(factory).supported).toBe(true);
    expect(factory).toEqual(before);
  });

  it("does not write any event history when compatibility fails", async () => {
    const sink = { write: vi.fn(async () => undefined) };
    const factory = factoryWith((candidate) => {
      candidate.orchestrator = { kind: "JAVASCRIPT", javascript: {} };
      candidate.workstations?.[0]?.resources?.push({
        name: "missing",
        capacity: 1,
      });
    });
    const result = await writeFactoryEventsIfCompatible(factory, [], sink);
    expect(result.supported).toBe(false);
    expect(sink.write).not.toHaveBeenCalled();
    if (!result.supported) expect(result.diagnostics.length).toBeGreaterThan(1);
  });

  it("writes one supplied batch only after a supported preflight", async () => {
    const sink = { write: vi.fn(async () => undefined) };
    const result = await writeFactoryEventsIfCompatible(
      supportedFactory,
      [],
      sink,
    );
    expect(result.supported).toBe(true);
    expect(sink.write).toHaveBeenCalledOnce();
  });
});
