import { buildCurrentActivityGraphLayoutFromFactory } from "../current-activity-factory-graph-layout";
import { loadFactoryGraphOnrejectionEdgeReproductionFactory } from "./factory-graph-onrejection-edge-reproduction.fixture";

describe("factory-graph-onrejection-edge-reproduction fixture", () => {
  it("loads a canonical factory with progress-outcome routes on standard processors", async () => {
    const factory = loadFactoryGraphOnrejectionEdgeReproductionFactory();

    expect(factory.name).toBe("factory");

    const process = factory.workstations?.find(
      (workstation) => workstation.name === "process",
    );
    const review = factory.workstations?.find(
      (workstation) => workstation.name === "review",
    );
    const plan = factory.workstations?.find(
      (workstation) => workstation.name === "plan",
    );

    expect(process?.onContinue?.length).toBeGreaterThan(0);
    expect(process?.onRejection?.length).toBeGreaterThan(0);
    expect(review?.onRejection?.length).toBeGreaterThan(0);
    expect(plan?.onRejection?.length).toBeGreaterThan(0);
    expect(process?.stopWords).toBeUndefined();
    expect(review?.stopWords).toBeUndefined();
    expect(plan?.stopWords).toBeUndefined();

    const processor = factory.workers?.find((worker) => worker.name === "processor");
    expect(processor?.stopToken).toBe("<COMPLETE>");
    expect(review?.type).toBe("MODEL_WORKSTATION");
    expect(review?.behavior).toBeUndefined();
  });

  it("builds current-activity graph layout from the committed fixture", async () => {
    const factory = loadFactoryGraphOnrejectionEdgeReproductionFactory();
    const layout = await buildCurrentActivityGraphLayoutFromFactory(factory);

    expect(layout.nodes.length).toBeGreaterThan(0);
    expect(layout.edges.length).toBeGreaterThan(0);
  });
});
