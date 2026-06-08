import { describe, expect, it } from "vitest";

import { baseFactoryDefinition } from "../draft/factory-graph-draft.test-helpers";
import type { CanonicalFactoryDefinition } from "../draft/factory-graph-draft-types";
import {
  buildFactoryGraphLayoutTopologyKey,
  doesFactoryDefinitionChangeAffectGraphTopology,
} from "../operations/factory-graph-topology-impact";

function cloneDefinition(
  definition: CanonicalFactoryDefinition = baseFactoryDefinition,
): CanonicalFactoryDefinition {
  return structuredClone(definition);
}

function firstWorkType(definition: CanonicalFactoryDefinition) {
  const workType = definition.workTypes?.[0];
  if (!workType) {
    throw new Error("expected work type fixture");
  }
  return workType;
}

function firstWorkstation(definition: CanonicalFactoryDefinition) {
  const workstation = definition.workstations?.[0];
  if (!workstation) {
    throw new Error("expected workstation fixture");
  }
  return workstation;
}

function firstWorker(definition: CanonicalFactoryDefinition) {
  const worker = definition.workers?.[0];
  if (!worker) {
    throw new Error("expected worker fixture");
  }
  return worker;
}

describe("doesFactoryDefinitionChangeAffectGraphTopology work types", () => {
  it("returns true when a work type is added", () => {
    const previous = cloneDefinition();
    const next = cloneDefinition();
    next.workTypes = [
      ...(next.workTypes ?? []),
      {
        name: "bug",
        states: [{ name: "open", type: "INITIAL" }],
      },
    ];

    expect(doesFactoryDefinitionChangeAffectGraphTopology(previous, next)).toBe(
      true,
    );
  });

  it("returns false when only handling behavior changes", () => {
    const previous = cloneDefinition();
    const next = cloneDefinition();
    next.workTypes = [
      {
        ...firstWorkType(previous),
        handlingBehavior: ["DEFAULT"],
      },
    ];

    expect(doesFactoryDefinitionChangeAffectGraphTopology(previous, next)).toBe(
      false,
    );
  });
});

describe("doesFactoryDefinitionChangeAffectGraphTopology work states", () => {
  it("returns true when a work state is added", () => {
    const previous = cloneDefinition();
    const workType = firstWorkType(previous);
    const next = cloneDefinition();
    next.workTypes = [
      {
        ...workType,
        states: [...workType.states, { name: "review", type: "PROCESSING" }],
      },
    ];

    expect(doesFactoryDefinitionChangeAffectGraphTopology(previous, next)).toBe(
      true,
    );
  });

  it("returns false when only a work state type changes", () => {
    const previous = cloneDefinition();
    const workType = firstWorkType(previous);
    const next = cloneDefinition();
    next.workTypes = [
      {
        ...workType,
        states: workType.states.map((state) =>
          state.name === "queued" ? { ...state, type: "PROCESSING" } : state,
        ),
      },
    ];

    expect(doesFactoryDefinitionChangeAffectGraphTopology(previous, next)).toBe(
      false,
    );
  });
});

describe("doesFactoryDefinitionChangeAffectGraphTopology workstations", () => {
  it("returns true when workstation worker assignment changes", () => {
    const previous = cloneDefinition();
    const next = cloneDefinition();
    next.workers = [
      ...(next.workers ?? []),
      {
        model: "gpt-5",
        name: "reviewer",
        type: "MODEL_WORKER",
      },
    ];
    next.workstations = [
      {
        ...firstWorkstation(previous),
        worker: "reviewer",
      },
    ];

    expect(doesFactoryDefinitionChangeAffectGraphTopology(previous, next)).toBe(
      true,
    );
  });

  it("returns false when only workstation prompt body changes", () => {
    const previous = cloneDefinition();
    const next = cloneDefinition();
    next.workstations = [
      {
        ...firstWorkstation(previous),
        body: "Updated workstation instructions.",
      },
    ];

    expect(doesFactoryDefinitionChangeAffectGraphTopology(previous, next)).toBe(
      false,
    );
  });
});

describe("doesFactoryDefinitionChangeAffectGraphTopology workers", () => {
  it("returns true when a worker gains a resource link", () => {
    const previous = cloneDefinition();
    const next = cloneDefinition();
    next.resources = [
      {
        capacity: 1,
        name: "gpu",
      },
    ];
    next.workers = [
      {
        ...firstWorker(previous),
        resources: [{ name: "gpu" }],
      },
    ];

    expect(doesFactoryDefinitionChangeAffectGraphTopology(previous, next)).toBe(
      true,
    );
  });

  it("returns false when only worker model changes", () => {
    const previous = cloneDefinition();
    const next = cloneDefinition();
    next.workers = [
      {
        ...firstWorker(previous),
        model: "gpt-5.2",
      },
    ];

    expect(doesFactoryDefinitionChangeAffectGraphTopology(previous, next)).toBe(
      false,
    );
  });
});

describe("doesFactoryDefinitionChangeAffectGraphTopology resources", () => {
  it("returns true when a resource is added", () => {
    const previous = cloneDefinition();
    const next = cloneDefinition();
    next.resources = [
      ...(next.resources ?? []),
      {
        capacity: 2,
        name: "disk",
      },
    ];

    expect(doesFactoryDefinitionChangeAffectGraphTopology(previous, next)).toBe(
      true,
    );
  });

  it("returns false when only resource capacity changes", () => {
    const previous = cloneDefinition();
    previous.resources = [
      {
        capacity: 2,
        name: "gpu",
      },
    ];
    const next = cloneDefinition(previous);
    next.resources = [
      {
        capacity: 8,
        name: "gpu",
      },
    ];

    expect(doesFactoryDefinitionChangeAffectGraphTopology(previous, next)).toBe(
      false,
    );
  });
});

describe("buildFactoryGraphLayoutTopologyKey", () => {
  it("returns the same key when only workstation prompt body changes", () => {
    const previous = cloneDefinition();
    const next = cloneDefinition();
    next.workstations = [
      {
        ...firstWorkstation(previous),
        body: "Updated workstation instructions.",
      },
    ];

    expect(buildFactoryGraphLayoutTopologyKey(previous)).toBe(
      buildFactoryGraphLayoutTopologyKey(next),
    );
  });
});
