import {
  findWorkerInFactoryDefinition,
  workstationNamesReferencingWorkerInFactoryDefinition,
} from "./worker-detail-values";

describe("worker-detail-values", () => {
  const factory = {
    name: "factory",
    workers: [
      {
        model: "gpt-5",
        modelProvider: "CODEX" as const,
        name: "reviewer",
        type: "MODEL_WORKER" as const,
      },
    ],
    workstations: [
      { id: "review", name: "Review", worker: "reviewer" },
      { id: "plan", name: "Plan", worker: "reviewer" },
      { id: "code", name: "Code", worker: "planner" },
    ],
    workTypes: [],
  };

  it("finds workers by authored name in the factory document", () => {
    expect(findWorkerInFactoryDefinition(factory, "reviewer")).toEqual({
      model: "gpt-5",
      modelProvider: "CODEX",
      name: "reviewer",
      type: "MODEL_WORKER",
    });
  });

  it("lists workstations that reference the worker", () => {
    expect(
      workstationNamesReferencingWorkerInFactoryDefinition(factory, "reviewer"),
    ).toEqual(["Review", "Plan"]);
  });

  it("returns undefined when the worker is missing from the factory document", () => {
    expect(findWorkerInFactoryDefinition(factory, "missing")).toBeUndefined();
  });

  it("omits workstations with blank names from the referencing list", () => {
    expect(
      workstationNamesReferencingWorkerInFactoryDefinition(
        {
          ...factory,
          workstations: [
            { id: "blank", name: "", worker: "reviewer" },
            { id: "review", name: "Review", worker: "reviewer" },
          ],
        },
        "reviewer",
      ),
    ).toEqual(["Review"]);
  });
});
