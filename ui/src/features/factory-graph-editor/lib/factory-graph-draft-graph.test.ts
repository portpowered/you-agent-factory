import { buildFactoryGraphTopologyFromDefinition } from "./factory-graph-draft-graph";
import type { CanonicalFactoryDefinition } from "./factory-graph-draft-types";

const runtimeShapedFactory: CanonicalFactoryDefinition = {
  name: "runtime-shaped-factory",
  workTypes: [
    {
      name: "story",
      states: [
        { name: "new", type: "INITIAL" },
        { name: "done", type: "TERMINAL" },
      ],
    },
    {
      name: "__system_time",
      states: [{ name: "pending", type: "PROCESSING" }],
    },
  ],
  workers: [
    {
      model: "gpt-5",
      name: "router",
      type: "MODEL_WORKER",
    },
  ],
  workstations: [
    {
      inputs: [{ state: "new", workType: "story" }],
      name: "route-story",
      outputs: [
        { state: "done", workType: "story" },
        { state: "pending", workType: "__system_time" },
      ],
      type: "MODEL_WORKSTATION",
      worker: "router",
    },
    {
      inputs: [{ state: "pending", workType: "__system_time" }],
      name: "__system_time:expire",
      outputs: [],
      type: "MODEL_WORKSTATION",
      worker: "",
    },
  ],
};

it("does not synthesize worker nodes or assignment edges for empty workstation worker names", () => {
  const topology =
    buildFactoryGraphTopologyFromDefinition(runtimeShapedFactory);

  expect(topology.nodes.map((node) => node.id)).toEqual(
    expect.arrayContaining([
      "worker:router",
      "workstation:route-story",
      "workstation:__system_time:expire",
    ]),
  );
  expect(topology.nodes.map((node) => node.id)).not.toContain("worker:");
  expect(topology.edges.map((edge) => edge.id)).toEqual(
    expect.arrayContaining([
      "worker-assignment:worker:router->workstation:route-story",
    ]),
  );
  expect(topology.edges.some((edge) => edge.kind === "worker-assignment")).toBe(
    true,
  );
  expect(
    topology.edges.filter((edge) => edge.kind === "worker-assignment"),
  ).toHaveLength(1);
});

it("treats whitespace-only workstation worker names as unassigned", () => {
  const topology = buildFactoryGraphTopologyFromDefinition({
    ...runtimeShapedFactory,
    workstations: [
      {
        inputs: [{ state: "new", workType: "story" }],
        name: "unassigned",
        outputs: [],
        type: "MODEL_WORKSTATION",
        worker: "   ",
      },
    ],
  });

  expect(topology.nodes.map((node) => node.id)).not.toContain("worker:");
  expect(topology.edges.map((edge) => edge.id)).not.toEqual(
    expect.arrayContaining([expect.stringMatching(/^worker-assignment:/)]),
  );
});

it("uses explicit entity ids for canonical graph node and edge ids", () => {
  const topology = buildFactoryGraphTopologyFromDefinition({
    name: "stable-ids",
    resources: [{ capacity: 1, id: "resource-slot", name: "slot" }],
    workers: [
      {
        id: "worker-reviewer",
        name: "reviewer",
        resources: [{ name: "slot" }],
        type: "MODEL_WORKER",
      },
    ],
    workTypes: [
      {
        id: "work-type-story",
        name: "story",
        states: [
          { id: "state-queued", name: "queued", type: "INITIAL" },
          { id: "state-done", name: "done", type: "TERMINAL" },
        ],
      },
    ],
    workstations: [
      {
        id: "workstation-review",
        inputs: [{ state: "queued", workType: "story" }],
        name: "review",
        outputs: [{ state: "done", workType: "story" }],
        resources: [{ name: "slot" }],
        type: "MODEL_WORKSTATION",
        worker: "reviewer",
      },
    ],
  });

  expect(topology.nodes.map((node) => node.id)).toEqual(
    expect.arrayContaining([
      "resource:resource-slot",
      "worker:worker-reviewer",
      "work-type:work-type-story",
      "work-state:work-type-story:state-queued",
      "work-state:work-type-story:state-done",
      "workstation:workstation-review",
    ]),
  );
  expect(topology.edges.map((edge) => edge.id)).toEqual(
    expect.arrayContaining([
      "worker-resource:resource:resource-slot->worker:worker-reviewer",
      "worker-assignment:worker:worker-reviewer->workstation:workstation-review",
      "workstation-resource:resource:resource-slot->workstation:workstation-review",
      "workstation-input:work-state:work-type-story:state-queued->workstation:workstation-review",
      "workstation-output:workstation:workstation-review->work-state:work-type-story:state-done",
    ]),
  );
});

it("keeps legacy fallback graph ids for name-keyed factories", () => {
  const topology = buildFactoryGraphTopologyFromDefinition({
    name: "legacy-name-keyed-factory",
    resources: [{ capacity: 1, name: "slot" }],
    workers: [
      {
        name: "reviewer",
        resources: [{ name: "slot" }],
        type: "MODEL_WORKER",
      },
    ],
    workTypes: [
      {
        name: "story",
        states: [{ name: "queued", type: "INITIAL" }],
      },
    ],
    workstations: [
      {
        inputs: [{ state: "queued", workType: "story" }],
        name: "review",
        outputs: [{ state: "queued", workType: "story" }],
        resources: [{ name: "slot" }],
        type: "MODEL_WORKSTATION",
        worker: "reviewer",
      },
    ],
  });

  expect(topology.nodes.map((node) => node.id)).toEqual(
    expect.arrayContaining([
      "resource:slot",
      "worker:reviewer",
      "work-type:story",
      "work-state:story:queued",
      "workstation:review",
    ]),
  );
  expect(topology.edges.map((edge) => edge.id)).toEqual(
    expect.arrayContaining([
      "worker-resource:resource:slot->worker:reviewer",
      "worker-assignment:worker:reviewer->workstation:review",
      "workstation-input:work-state:story:queued->workstation:review",
      "workstation-output:workstation:review->work-state:story:queued",
    ]),
  );
});

it("keeps canonical graph ids stable across renames for id-backed factories", () => {
  const beforeRename = buildFactoryGraphTopologyFromDefinition({
    name: "stable-before-rename",
    resources: [{ capacity: 1, id: "resource-slot", name: "slot" }],
    workers: [
      {
        id: "worker-reviewer",
        name: "reviewer",
        resources: [{ name: "slot" }],
        type: "MODEL_WORKER",
      },
    ],
    workTypes: [
      {
        id: "work-type-story",
        name: "story",
        states: [{ id: "state-queued", name: "queued", type: "INITIAL" }],
      },
    ],
    workstations: [
      {
        id: "workstation-review",
        inputs: [{ state: "queued", workType: "story" }],
        name: "review",
        outputs: [{ state: "queued", workType: "story" }],
        resources: [{ name: "slot" }],
        type: "MODEL_WORKSTATION",
        worker: "reviewer",
      },
    ],
  });

  const afterRename = buildFactoryGraphTopologyFromDefinition({
    name: "stable-after-rename",
    resources: [{ capacity: 1, id: "resource-slot", name: "renamed-slot" }],
    workers: [
      {
        id: "worker-reviewer",
        name: "renamed-reviewer",
        resources: [{ name: "renamed-slot" }],
        type: "MODEL_WORKER",
      },
    ],
    workTypes: [
      {
        id: "work-type-story",
        name: "renamed-story",
        states: [
          { id: "state-queued", name: "renamed-queued", type: "INITIAL" },
        ],
      },
    ],
    workstations: [
      {
        id: "workstation-review",
        inputs: [{ state: "renamed-queued", workType: "renamed-story" }],
        name: "renamed-review",
        outputs: [{ state: "renamed-queued", workType: "renamed-story" }],
        resources: [{ name: "renamed-slot" }],
        type: "MODEL_WORKSTATION",
        worker: "renamed-reviewer",
      },
    ],
  });

  expect(afterRename.nodes.map((node) => node.id)).toEqual(
    beforeRename.nodes.map((node) => node.id),
  );
  expect(afterRename.edges.map((edge) => edge.id)).toEqual(
    beforeRename.edges.map((edge) => edge.id),
  );
});
