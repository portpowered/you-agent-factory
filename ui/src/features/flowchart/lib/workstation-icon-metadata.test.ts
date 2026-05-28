// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: workstation-kind regression matrix stays grouped around one metadata fixture.
import type { DashboardWorkstationNode } from "../../../api/dashboard/types";
import {
  CRON_WORKSTATION_KIND,
  POLLER_WORKSTATION_KIND,
  REPEATER_WORKSTATION_KIND,
  STANDARD_WORKSTATION_KIND,
  SUPPORTED_WORKSTATION_ICON_METADATA,
  workstationIconMetadata,
  workstationSemanticKind,
} from "./workstation-icon-metadata";
import { EXHAUSTION_WORKSTATION_KIND } from "./workstation-semantics";

function dashboardWorkstationNode(
  overrides: Partial<DashboardWorkstationNode> = {},
): DashboardWorkstationNode {
  return {
    node_id: "node-1",
    transition_id: "transition-1",
    workstation_kind: STANDARD_WORKSTATION_KIND,
    workstation_name: "Plan",
    worker_type: "processor",
    ...overrides,
  };
}

describe("workstationIconMetadata", () => {
  it("publishes the approved dashboard workstation icon vocabulary for supported kinds", () => {
    expect(SUPPORTED_WORKSTATION_ICON_METADATA).toEqual([
      {
        className: "text-af-text-subtle",
        iconKind: "workstation",
        label: "Standard workstation",
        semanticKind: STANDARD_WORKSTATION_KIND,
      },
      {
        className: "text-af-info",
        iconKind: "repeater",
        label: "Repeater workstation",
        semanticKind: REPEATER_WORKSTATION_KIND,
      },
      {
        className: "text-af-success",
        iconKind: "cron",
        label: "Cron workstation",
        semanticKind: CRON_WORKSTATION_KIND,
      },
      {
        className: "text-af-accent",
        iconKind: "poller",
        label: "Poller workstation",
        semanticKind: POLLER_WORKSTATION_KIND,
      },
    ]);
  });

  it("maps canonical workstation kinds to one shared semantic icon contract", () => {
    expect(
      workstationIconMetadata(
        dashboardWorkstationNode({
          workstation_kind: STANDARD_WORKSTATION_KIND,
        }),
      ),
    ).toEqual({
      className: "text-af-text-subtle",
      iconKind: "workstation",
      label: "Standard workstation",
      semanticKind: STANDARD_WORKSTATION_KIND,
    });
    expect(
      workstationIconMetadata(
        dashboardWorkstationNode({
          workstation_kind: REPEATER_WORKSTATION_KIND,
        }),
      ),
    ).toEqual({
      className: "text-af-info",
      iconKind: "repeater",
      label: "Repeater workstation",
      semanticKind: REPEATER_WORKSTATION_KIND,
    });
    expect(
      workstationIconMetadata(
        dashboardWorkstationNode({ workstation_kind: CRON_WORKSTATION_KIND }),
      ),
    ).toEqual({
      className: "text-af-success",
      iconKind: "cron",
      label: "Cron workstation",
      semanticKind: CRON_WORKSTATION_KIND,
    });
    expect(
      workstationIconMetadata(
        dashboardWorkstationNode({ workstation_kind: POLLER_WORKSTATION_KIND }),
      ),
    ).toEqual({
      className: "text-af-accent",
      iconKind: "poller",
      label: "Poller workstation",
      semanticKind: POLLER_WORKSTATION_KIND,
    });
  });

  it("normalizes legacy lowercase workstation kinds to the API enum icon contract", () => {
    expect(
      workstationIconMetadata(
        dashboardWorkstationNode({ workstation_kind: "repeater" }),
      ),
    ).toEqual({
      className: "text-af-info",
      iconKind: "repeater",
      label: "Repeater workstation",
      semanticKind: REPEATER_WORKSTATION_KIND,
    });
  });

  it("preserves the exhaustion-rule special case ahead of workstation-kind fallback", () => {
    const explicitExhaustion = dashboardWorkstationNode({
      workstation_kind: EXHAUSTION_WORKSTATION_KIND,
      worker_type: "processor",
    });
    const emptyFallbackExhaustion = dashboardWorkstationNode({
      workstation_kind: "",
      worker_type: "",
    });

    expect(workstationSemanticKind(explicitExhaustion)).toBe(
      EXHAUSTION_WORKSTATION_KIND,
    );
    expect(workstationSemanticKind(emptyFallbackExhaustion)).toBe(
      EXHAUSTION_WORKSTATION_KIND,
    );
    expect(workstationIconMetadata(explicitExhaustion)).toEqual({
      className: "text-af-danger",
      iconKind: "exhaustion",
      label: "Exhaustion rule",
      semanticKind: EXHAUSTION_WORKSTATION_KIND,
    });
  });

  it("treats unknown workstation kinds as the standard workstation icon", () => {
    expect(
      workstationIconMetadata(
        dashboardWorkstationNode({ workstation_kind: "future-kind" }),
      ),
    ).toEqual({
      className: "text-af-text-subtle",
      iconKind: "workstation",
      label: "Standard workstation",
      semanticKind: STANDARD_WORKSTATION_KIND,
    });
  });

  it("localizes workstation semantic labels without changing icon contracts", () => {
    expect(
      workstationIconMetadata(
        dashboardWorkstationNode({
          workstation_kind: REPEATER_WORKSTATION_KIND,
        }),
        "zh-CN",
      ),
    ).toEqual({
      className: "text-af-info",
      iconKind: "repeater",
      label: "重复器工作站",
      semanticKind: REPEATER_WORKSTATION_KIND,
    });
  });
});
