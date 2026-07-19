export function createDenseFactoryRecording(): unknown {
  const resources = [
    { capacity: 4, name: "compute-pool" },
    { capacity: 2, name: "review-slots" },
  ];
  const workers = [
    {
      name: "intake-agent",
      resources: [{ capacity: 1, name: "compute-pool" }],
    },
    {
      name: "planning-agent",
      resources: [{ capacity: 1, name: "compute-pool" }],
    },
    {
      name: "delivery-agent",
      resources: [{ capacity: 1, name: "compute-pool" }],
    },
    {
      name: "review-agent",
      resources: [{ capacity: 1, name: "review-slots" }],
    },
  ];
  const workTypes = ["request", "plan", "delivery"].map((name) => ({
    name,
    states: [
      { name: "queued", type: "INITIAL" },
      { name: "active", type: "PROCESSING" },
      { name: "complete", type: "TERMINAL" },
      { name: "failed", type: "FAILED" },
    ],
  }));
  const workstations = [
    workstation("Triage", "intake-agent", "request", "queued", "active"),
    workstation("Plan", "planning-agent", "request", "active", "complete"),
    workstation("Draft", "planning-agent", "plan", "queued", "active"),
    workstation("Review", "review-agent", "plan", "active", "complete"),
    workstation("Build", "delivery-agent", "delivery", "queued", "active"),
    workstation("Verify", "review-agent", "delivery", "active", "complete"),
  ];
  const factory = {
    name: "dense-local-factory",
    resources,
    workers,
    workTypes,
    workstations,
  };
  const context = (sequence: number, dispatchId?: string) => ({
    ...(dispatchId ? { dispatchId, workIds: ["request-1"] } : {}),
    eventTime: `2026-07-18T21:00:${String(sequence).padStart(2, "0")}Z`,
    sequence,
    sessionId: "storybook-dense-session",
    sessionSequence: sequence,
    tick: sequence === 1 ? 6_999 : 7_000,
  });

  return {
    events: [
      {
        context: context(1),
        id: "dense-topology",
        payload: { factory },
        schemaVersion: "agent-factory.event.v1",
        type: "INITIAL_STRUCTURE_REQUEST",
      },
      {
        context: context(2),
        id: "dense-work",
        payload: {
          type: "FACTORY_REQUEST_BATCH",
          works: [
            {
              name: "Customer request 1",
              workId: "request-1",
              workTypeName: "request",
            },
            {
              name: "Customer request 2",
              workId: "request-2",
              workTypeName: "request",
            },
          ],
        },
        schemaVersion: "agent-factory.event.v1",
        type: "WORK_REQUEST",
      },
      {
        context: context(3, "dispatch-dense-1"),
        id: "dense-dispatch",
        payload: {
          inputs: [{ workId: "request-1" }],
          resources: [{ capacity: 1, name: "compute-pool" }],
          transitionId: "Triage",
        },
        schemaVersion: "agent-factory.event.v1",
        type: "DISPATCH_REQUEST",
      },
    ],
    factory,
    id: "dense-recording",
    schemaVersion: "factory-recording/v1",
    title: "Dense local Factory recording",
  };
}

function workstation(
  name: string,
  worker: string,
  workType: string,
  inputState: string,
  outputState: string,
) {
  return {
    inputs: [{ state: inputState, workType }],
    name,
    onFailure: [{ state: "failed", workType }],
    outputs: [{ state: outputState, workType }],
    resources: [
      {
        capacity: 1,
        name: worker === "review-agent" ? "review-slots" : "compute-pool",
      },
    ],
    worker,
  };
}
