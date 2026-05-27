import { describe, expect, it } from "vitest";
import { currentSelectionDetailMessagesByLocale } from "../features/current-selection/messages/current-selection-detail";
import { currentSelectionDispatchHistoryMessagesByLocale } from "../features/current-selection/messages/current-selection-dispatch-history";
import { currentSelectionShellMessagesByLocale } from "../features/current-selection/messages/current-selection-shell";
import { providerSessionDetailMessagesByLocale } from "../features/provider-session-detail/messages/provider-session-detail";
import { workstationDetailMessagesByLocale } from "../features/current-selection/messages/workstation-detail";
import { exportDialogMessagesByLocale } from "../features/export/messages/export-dialog";
import { headerControlsMessagesByLocale } from "../features/header/messages/header-controls";
import { importPreviewDialogMessagesByLocale } from "../features/import/messages/import-preview-dialog";
import { terminalWorkMessagesByLocale } from "../features/terminal-work/messages/terminal-work";
import { dashboardFlowAxisLegendMessagesByLocale } from "../features/workflow-activity/messages/dashboard-flow-axis-legend";
import { workflowActivityGraphImportMessagesByLocale } from "../features/workflow-activity/messages/graph-import";
import {
  resolveLocalizedMessages,
  validateRequiredLocaleMessages,
} from "./messages";

const featureCatalogs = {
  currentSelectionDetail: currentSelectionDetailMessagesByLocale,
  currentSelectionDispatchHistory:
    currentSelectionDispatchHistoryMessagesByLocale,
  currentSelectionProviderSessionDetail: providerSessionDetailMessagesByLocale,
  currentSelectionShell: currentSelectionShellMessagesByLocale,
  dashboardFlowAxisLegend: dashboardFlowAxisLegendMessagesByLocale,
  exportDialog: exportDialogMessagesByLocale,
  graphImport: workflowActivityGraphImportMessagesByLocale,
  headerControls: headerControlsMessagesByLocale,
  importPreviewDialog: importPreviewDialogMessagesByLocale,
  terminalWork: terminalWorkMessagesByLocale,
  workstationDetail: workstationDetailMessagesByLocale,
};

describe("resolveLocalizedMessages", () => {
  it("falls back to English when the requested locale catalog is missing", () => {
    const messages = {
      en: {
        title: "Dashboard",
      },
    };

    expect(resolveLocalizedMessages(messages, "zh-CN")).toEqual({
      title: "Dashboard",
    });
  });
});

describe("validateRequiredLocaleMessages", () => {
  it.each(
    Object.entries(featureCatalogs),
  )("finds complete required locale messages for %s", (_catalogName, catalog) => {
    expect(validateRequiredLocaleMessages(catalog)).toEqual([]);
  });

  it("reports missing nested fields from required locale catalogs", () => {
    const messages = {
      en: {
        errors: {
          unavailable: "Unavailable",
        },
        title: "Dashboard",
      },
      "zh-CN": {
        errors: {},
      },
    };

    expect(validateRequiredLocaleMessages(messages)).toEqual([
      {
        locale: "zh-CN",
        path: "zh-CN.errors.unavailable",
      },
      {
        locale: "zh-CN",
        path: "zh-CN.title",
      },
    ]);
  });
});
