import "@testing-library/jest-dom/vitest";
import "../../../../testing/react-flow-current-activity-card-component.mocks";

import {
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import type {
  DashboardSnapshot,
  DashboardWorkItemRef,
} from "../../../../api/dashboard/types";
import {
  type ImportFactoryValue,
  SessionFactoryAPIError,
} from "../../../../api/session-factory";
import {
  semanticWorkflowDashboardSnapshot,
  singleNodeDashboardSnapshot,
  twentyNodeDashboardSnapshot,
} from "../../../../components/dashboard/test-fixtures";
import type { ReadFactoryImportFile } from "../../../import/hooks/use-factory-png-drop";
import type { FactoryImportConfirmInput } from "../../../import/lib/factory-import-save-choice";
import type { FactoryPngImportValue } from "../../../import/lib/factory-png-import";
import { getImportPreviewDialogMessages } from "../../../import/messages/import-preview-dialog";
import { currentActivityTopologyKey } from "../../lib/react-flow-current-activity-card-keys";
import { getDashboardFlowAxisLegendMessages } from "../../messages/dashboard-flow-axis-legend";
import { getWorkflowActivityGraphImportMessages } from "../../messages/graph-import";
import {
  createFactoryImportValue,
  createFileDropTransfer,
  expandGraphLegend,
  refreshFactoryFromTopology,
  registerCurrentActivityCardTestLifecycle,
  renderCurrentActivity,
} from "../../../../testing/react-flow-current-activity-card-component.harness";

const workflowGraphLocaleFallbackTimeoutMs = 180_000;

function dashboardSnapshotWithActiveWorkLabels(
  labels: string[],
  workstationName?: string,
): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
  const activeExecution =
    snapshot.runtime.active_executions_by_dispatch_id?.[
      "dispatch-review-active"
    ];
  const reviewWorkstation = snapshot.topology.workstation_nodes_by_id.review;

  if (reviewWorkstation && workstationName) {
    reviewWorkstation.workstation_name = workstationName;
  }

  if (activeExecution) {
    activeExecution.work_items = labels.map(
      (label, index): DashboardWorkItemRef => {
        const itemNumber = index + 1;

        return {
          display_name: label,
          trace_id: `trace-active-story-${itemNumber}`,
          work_id: `work-active-story-${itemNumber}`,
          work_type_id: "story",
        };
      },
    );
    activeExecution.trace_ids = activeExecution.work_items.map(
      (workItem) => workItem.trace_id ?? workItem.work_id,
    );
  }

  return refreshFactoryFromTopology(snapshot);
}

function dashboardSnapshotWithLongWorkstationAndActiveWorkLabels(): DashboardSnapshot {
  return dashboardSnapshotWithActiveWorkLabels(
    [
      "Short Active Story",
      "Active Story With A Medium Sized Label",
      "Active Story With A Deliberately Long Label That Must Stay Inside The Workstation Node",
    ],
    "Review Requests With A Deliberately Long Workstation Title",
  );
}

describe("ReactFlowCurrentActivityCard topology selection and localization", () => {
  registerCurrentActivityCardTestLifecycle();

  it("derives a stable topology cache key for equivalent cloned workflow topology", () => {
    const firstKey = currentActivityTopologyKey(
      semanticWorkflowDashboardSnapshot.topology,
    );
    const secondKey = currentActivityTopologyKey(
      structuredClone(semanticWorkflowDashboardSnapshot).topology,
    );

    expect(secondKey).toBe(firstKey);
  });

  it("selects work-state nodes and exposes resource nodes as resource selectors", async () => {
    const { onSelectResource, onSelectStateNode } = renderCurrentActivity({
      snapshot: structuredClone(semanticWorkflowDashboardSnapshot),
      selection: { kind: "state-node", placeId: "story:implemented" },
    });

    const stateButton = await screen.findByRole("button", {
      name: "Select story:implemented state",
    });

    expect(stateButton.getAttribute("aria-pressed")).toBe("true");
    expect(stateButton.getAttribute("data-selected-state")).toBe("true");
    expect(
      screen.queryByRole("button", {
        name: "Select agent-slot state",
      }),
    ).toBeNull();
    expect(
      screen.getByRole("button", { name: "Select agent-slot resource" }),
    ).toBeTruthy();

    fireEvent.click(
      await screen.findByRole("button", { name: "Select story:ready state" }),
    );

    expect(onSelectStateNode).toHaveBeenCalledWith("story:ready");

    fireEvent.click(
      screen.getByRole("button", { name: "Select agent-slot resource" }),
    );

    expect(onSelectResource).toHaveBeenCalledWith("agent-slot");
  });

  it("keeps long workstation and active work labels from hiding the duration", async () => {
    const labels = [
      "Short Active Story",
      "Active Story With A Medium Sized Label",
      "Active Story With A Deliberately Long Label That Must Stay Inside The Workstation Node",
    ];
    const { onSelectWorkID } = renderCurrentActivity({
      snapshot: dashboardSnapshotWithLongWorkstationAndActiveWorkLabels(),
    });
    const longWorkstationButton = await screen.findByRole("button", {
      name: /Select Review Requests With A Deliberately Long Workstation Title workstation/,
    });
    const longWorkstationLabel = longWorkstationButton.querySelector(
      "[data-workstation-title]",
    );
    const longWorkButton = await screen.findByRole("button", {
      name: /Active Story With A Deliberately Long Label/,
    });
    const longWorkLabel = longWorkButton.querySelector(
      "[data-active-work-label]",
    );
    const durationLabel = longWorkButton.querySelector(
      "[data-active-work-duration]",
    );
    const reviewNode = longWorkButton.closest(".react-flow__node");

    expect(reviewNode?.getAttribute("style")).toContain("width: 156px");
    expect(longWorkstationButton.getAttribute("title")).toBe(
      "Review Requests With A Deliberately Long Workstation Title",
    );
    expect(longWorkstationLabel?.className).toContain("truncate");
    expect(longWorkstationLabel?.className).toContain("whitespace-nowrap");
    expect(longWorkButton.className).toContain("min-w-0");
    expect(longWorkButton.className).toContain(
      "grid-cols-[minmax(0,1fr)_auto]",
    );
    expect(longWorkButton.className).toContain("overflow-hidden");
    expect(longWorkLabel?.className).toContain("truncate");
    expect(longWorkLabel?.className).toContain("basis-0");
    expect(durationLabel?.textContent).toBe("4s");
    expect(durationLabel?.className).toContain("whitespace-nowrap");
    expect(durationLabel?.className).toContain("text-right");
    expect(durationLabel?.className).not.toContain("overflow-hidden");
    labels.forEach((label) => {
      const activeWorkButton = screen.getByRole("button", {
        name: new RegExp(label),
      });
      const labelElement = activeWorkButton.querySelector(
        "[data-active-work-label]",
      );
      const durationElement = activeWorkButton.querySelector(
        "[data-active-work-duration]",
      );

      expect(labelElement?.textContent).toBe(label);
      expect(labelElement?.className).toContain("min-w-0");
      expect(labelElement?.className).toContain("truncate");
      expect(durationElement?.textContent).toBe("4s");
      expect(durationElement?.className).toContain("shrink-0");
      expect(durationElement?.className).toContain("whitespace-nowrap");
    });
    expect(longWorkButton.getAttribute("aria-pressed")).toBe("false");

    fireEvent.click(longWorkButton);

    await waitFor(() => {
      expect(onSelectWorkID).toHaveBeenCalled();
    });
    expect(onSelectWorkID).toHaveBeenCalledWith("work-active-story-3", {
      dispatchID: "dispatch-review-active",
      nodeID: "review",
    });

    cleanup();
    renderCurrentActivity({
      snapshot: dashboardSnapshotWithLongWorkstationAndActiveWorkLabels(),
      selection: {
        dispatchId: "dispatch-review-active",
        kind: "work-item",
        nodeId: "review",
        workID: "work-active-story-3",
      },
    });

    expect(
      (
        await screen.findByRole("button", {
          name: /Active Story With A Deliberately Long Label/,
        })
      ).getAttribute("aria-pressed"),
    ).toBe("true");
  });

  it("renders a safe fallback label when an active work item is missing both display name and work id", async () => {
    const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
    const activeExecution =
      snapshot.runtime.active_executions_by_dispatch_id?.[
        "dispatch-review-active"
      ];

    if (!activeExecution) {
      throw new Error(
        "expected semantic workflow fixture to include an active review execution",
      );
    }

    activeExecution.work_items = [
      {
        trace_id: "trace-malformed-active-story",
        work_type_id: "story",
      } as DashboardWorkItemRef,
    ];
    activeExecution.trace_ids = ["trace-malformed-active-story"];

    renderCurrentActivity({ snapshot });

    expect(
      await screen.findByRole("button", { name: "Select Review workstation" }),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: /Unknown work/ })).toBeTruthy();
  });

  it("renders a single-node workflow without edge data", async () => {
    renderCurrentActivity({ snapshot: singleNodeDashboardSnapshot });

    expect(
      await screen.findByRole("button", { name: "Select Intake workstation" }),
    ).toBeTruthy();
    expect(screen.queryByText("Idle")).toBeNull();
  });

  it("renders a twenty-node workflow fixture for larger graphs", async () => {
    const legendMessages = getDashboardFlowAxisLegendMessages("en");

    renderCurrentActivity({ snapshot: twentyNodeDashboardSnapshot });

    expect(
      await screen.findByRole("button", {
        name: "Select Station 20 workstation",
      }),
    ).toBeTruthy();
    expect(
      screen.getAllByRole("button", { name: /Select .* workstation/ }),
    ).toHaveLength(20);
    expect(
      screen.getAllByRole("img", { name: "Standard workstation" }).length,
    ).toBeGreaterThan(0);
    expect(
      screen
        .getAllByRole("img", { name: "Queue" })[0]
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("queue");
    const _legend = await expandGraphLegend();
    expect(
      within(screen.getByLabelText(legendMessages.title))
        .getByRole("img", {
          name: legendMessages.iconLabel(legendMessages.iconLabels.workstation),
        })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("workstation");
    expect(screen.queryByText("Workstation Definition")).toBeNull();
    expect(
      screen.getByRole("button", { name: "Select story:step-6 state" }),
    ).toBeTruthy();
  });

  it("renders localized legend copy for the workflow graph without changing the graph interactions", async () => {
    const messages = getDashboardFlowAxisLegendMessages("ja");

    renderCurrentActivity({
      snapshot: semanticWorkflowDashboardSnapshot,
      locale: "ja",
    });

    const expandButton = await screen.findByRole("button", {
      name: messages.expandToggleLabel(messages.title),
    });

    fireEvent.click(expandButton);

    const legend = await screen.findByLabelText(messages.title);

    expect(
      within(legend).getByText(messages.edgeLabels.activeFlow),
    ).toBeTruthy();
    expect(
      within(legend)
        .getByRole("img", {
          name: messages.iconLabel(messages.iconLabels.workstation),
        })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("workstation");
    expect(
      await screen.findByRole("button", { name: "Select Review workstation" }),
    ).toBeTruthy();
  });

  it("renders localized graph-import overlay and preview dialog shell copy without changing import behavior", async () => {
    const graphMessages = getWorkflowActivityGraphImportMessages("ja");
    const previewMessages = getImportPreviewDialogMessages("ja");
    const file = new File(["png"], "factory-import.png", { type: "image/png" });
    const importValue = createFactoryImportValue();
    const onFactoryImportReady =
      vi.fn<(value: FactoryPngImportValue, file: File) => void>();
    const readFactoryImportFile = vi
      .fn<ReadFactoryImportFile>()
      .mockResolvedValue({
        ok: true,
        value: importValue,
      });

    renderCurrentActivity({
      locale: "ja",
      onFactoryImportReady,
      readFactoryImportFile,
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    const viewport = await screen.findByRole("region", {
      name: "Work graph viewport",
    });

    fireEvent.dragOver(viewport, createFileDropTransfer([file]));

    expect(screen.getByText(graphMessages.graphDropTitle)).toBeTruthy();
    expect(screen.getByText(graphMessages.graphDropHint)).toBeTruthy();

    fireEvent.drop(viewport, createFileDropTransfer([file]));

    const previewDialog = await screen.findByRole("dialog", {
      name: previewMessages.title,
    });

    expect(
      within(previewDialog).getByRole("button", {
        name: previewMessages.closeLabel,
      }),
    ).toBeTruthy();
  });

  it("renders localized graph-import validation errors and preserves dismiss reset behavior", async () => {
    const graphMessages = getWorkflowActivityGraphImportMessages("ja");
    const file = new File(["png"], "invalid-factory.png", {
      type: "image/png",
    });
    const onFactoryImportReady =
      vi.fn<(value: FactoryPngImportValue, file: File) => void>();
    const readFactoryImportFile = vi
      .fn<ReadFactoryImportFile>()
      .mockResolvedValue({
        error: {
          code: "PNG_METADATA_MISSING",
          message:
            "The selected PNG does not contain you-agent-factory factory metadata.",
        },
        ok: false,
      });

    renderCurrentActivity({
      locale: "ja",
      onFactoryImportReady,
      readFactoryImportFile,
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    const viewport = await screen.findByRole("region", {
      name: "Work graph viewport",
    });

    fireEvent.drop(viewport, createFileDropTransfer([file]));

    const alert = await screen.findByRole("alert");

    expect(alert.textContent).toContain(graphMessages.graphImportErrorTitle);
    expect(alert.textContent).toContain("invalid-factory.png");
    expect(alert.textContent).toContain(
      graphMessages.importErrorMetadataMissing,
    );
    expect(onFactoryImportReady).not.toHaveBeenCalled();
    expect(
      screen.queryByRole("dialog", { name: "Review factory import" }),
    ).toBeNull();
    expect(viewport.getAttribute("data-current-activity-drop-state")).toBe(
      "error",
    );

    fireEvent.click(
      screen.getByRole("button", { name: graphMessages.dismissAction }),
    );

    await waitFor(() => {
      expect(screen.queryByRole("alert")).toBeNull();
    });
    expect(viewport.getAttribute("data-current-activity-drop-state")).toBe(
      "idle",
    );
  });

  it("keeps localized preview dialog shell controls available when activation fails", async () => {
    const graphMessages = getWorkflowActivityGraphImportMessages("ja");
    const previewMessages = getImportPreviewDialogMessages("ja");
    const file = new File(["png"], "factory-import.png", { type: "image/png" });
    const importValue = createFactoryImportValue();
    const activateFactory = vi
      .fn<(input: FactoryImportConfirmInput) => Promise<ImportFactoryValue>>()
      .mockRejectedValue(
        new SessionFactoryAPIError("Named factory already exists.", {
          code: "FACTORY_ALREADY_EXISTS",
          status: 409,
        }),
      );
    const onFactoryActivated = vi.fn<() => void>();
    const readFactoryImportFile = vi
      .fn<ReadFactoryImportFile>()
      .mockResolvedValue({
        ok: true,
        value: importValue,
      });

    renderCurrentActivity({
      activateFactory,
      locale: "ja",
      onFactoryActivated,
      readFactoryImportFile,
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    const viewport = await screen.findByRole("region", {
      name: "Work graph viewport",
    });

    fireEvent.drop(viewport, createFileDropTransfer([file]));

    const previewDialog = await screen.findByRole("dialog", {
      name: previewMessages.title,
    });

    expect(
      within(previewDialog).getByText(graphMessages.dialogFlowLabel),
    ).toBeTruthy();

    fireEvent.click(
      within(previewDialog).getByRole("button", {
        name: previewMessages.activateAction,
      }),
    );

    const alert = await within(previewDialog).findByRole("alert");

    expect(alert.textContent).toContain(previewMessages.activationErrorTitle);
    expect(alert.textContent).toContain(
      previewMessages.errorByCode.FACTORY_ALREADY_EXISTS,
    );
    expect(onFactoryActivated).not.toHaveBeenCalled();
    expect(
      screen.getByRole("dialog", { name: previewMessages.title }),
    ).toBeTruthy();
    expect(
      within(previewDialog).getByRole("button", {
        name: previewMessages.closeLabel,
      }),
    ).toBeTruthy();
    expect(importValue.revokePreviewImageSrc).not.toHaveBeenCalled();
  });

  it("falls back to English graph-import copy for unsupported locales", async () => {
    const graphMessages = getWorkflowActivityGraphImportMessages("en");
    const file = new File(["png"], "factory-import.png", { type: "image/png" });

    renderCurrentActivity({
      locale: "fr-CA",
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    const viewport = await screen.findByRole("region", {
      name: "Work graph viewport",
    });

    fireEvent.dragOver(viewport, createFileDropTransfer([file]));

    expect(screen.getByText(graphMessages.graphDropTitle)).toBeTruthy();
    expect(screen.getByText(graphMessages.graphDropHint)).toBeTruthy();
  });

  it(
    "falls back to English legend copy for unsupported workflow graph locales",
    async () => {
      const messages = getDashboardFlowAxisLegendMessages("en");

      renderCurrentActivity({
        snapshot: semanticWorkflowDashboardSnapshot,
        locale: "fr-CA",
      });

      const legend = await expandGraphLegend("fr-CA");

      expect(
        within(legend).getByText(messages.edgeLabels.activeFlow),
      ).toBeTruthy();
      expect(
        within(legend)
          .getByRole("img", {
            name: messages.iconLabel(messages.iconLabels.queue),
          })
          .getAttribute("data-graph-semantic-icon"),
      ).toBe("queue");
      expect(
        await screen.findByRole("button", {
          name: "Select Review workstation",
        }),
      ).toBeTruthy();
    },
    workflowGraphLocaleFallbackTimeoutMs,
  );
});
