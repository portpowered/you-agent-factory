import {
  CRON_WORKSTATION_KIND,
  POLLER_WORKSTATION_KIND,
  REPEATER_WORKSTATION_KIND,
  STANDARD_WORKSTATION_KIND,
} from "./workstation-icon-metadata";
import {
  workstationBehaviorSemanticKind,
  workstationGraphBorderClassName,
  workstationGraphPresentation,
  workstationGraphPresentationFromBehavior,
} from "./workstation-graph-presentation";

describe("workstationGraphPresentation", () => {
  it("derives poller presentation from canonical workstation kind without UI-only flags", () => {
    expect(
      workstationGraphPresentation({
        node_id: "linear-poller",
        transition_id: "linear-poller",
        workstation_kind: "poller",
        workstation_name: "Linear Poller",
        worker_type: "hosted-worker",
      }),
    ).toEqual({
      borderClassName: "border-dotted",
      className: "text-primary",
      iconKind: "poller",
      label: "Poller workstation",
      semanticKind: POLLER_WORKSTATION_KIND,
    });
  });

  it("maps editable workstation behavior to the same semantic presentation contract", () => {
    expect(workstationBehaviorSemanticKind("POLLER")).toBe(
      POLLER_WORKSTATION_KIND,
    );
    expect(workstationBehaviorSemanticKind("REPEATER")).toBe(
      REPEATER_WORKSTATION_KIND,
    );
    expect(workstationBehaviorSemanticKind("CRON")).toBe(CRON_WORKSTATION_KIND);
    expect(workstationBehaviorSemanticKind("STANDARD")).toBe(
      STANDARD_WORKSTATION_KIND,
    );
    expect(
      workstationGraphPresentationFromBehavior("POLLER").iconKind,
    ).toBe("poller");
  });

  it("publishes distinct border treatments for supported workstation behaviors", () => {
    expect(workstationGraphBorderClassName(REPEATER_WORKSTATION_KIND)).toBe(
      "border-double",
    );
    expect(workstationGraphBorderClassName(POLLER_WORKSTATION_KIND)).toBe(
      "border-dotted",
    );
    expect(workstationGraphBorderClassName(CRON_WORKSTATION_KIND)).toBe(
      "border-dashed",
    );
    expect(workstationGraphBorderClassName(STANDARD_WORKSTATION_KIND)).toBe(
      undefined,
    );
  });
});
