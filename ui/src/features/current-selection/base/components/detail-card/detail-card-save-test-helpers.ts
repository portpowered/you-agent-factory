import { fireEvent, screen, within } from "@testing-library/react";
import type { Mock } from "vitest";

import type { CurrentFactoryDocument } from "../../../../../api/current-factory-definition";
import { createDeferredPromise } from "../../../../../testing/app-shell-export-test-utils";
import {
  type MockFactoryDocumentSaveReturn,
  mockFactoryDocumentSave,
} from "../../../../../testing/factory-document-save-mocks";
import type { FactoryDocumentSaveInput } from "../../../../current-factory-definition/hooks/useFactoryDocumentSave";
import type { CurrentSelectionState } from "../../../hooks/core/useCurrentSelection";
import { DETAIL_CARD_NOW } from "./detail-card-test-helpers";

export { DETAIL_CARD_NOW };

export const DETAIL_CARD_SAVE_FACTORY_VERSION = {
  logical: "7",
  physical: "2026-05-23T15:52:00Z",
} as const;

let latestDetailCardCurrentFactoryDocument: CurrentFactoryDocument | null =
  null;

function resolveLatestDetailCardCurrentFactoryDocument() {
  return latestDetailCardCurrentFactoryDocument;
}

export type DetailCardEditableFactoryDocumentOverrides = {
  behavior?: "STANDARD" | "REPEATER" | "POLLER";
  model?: string;
  modelProvider?:
    | "CURSOR"
    | "CODEX"
    | "CLAUDE"
    | "GEMINI"
    | "KIRO"
    | "OPENCODE";
  prompt?: string;
  workerName?: string;
  workerOptions?: string[];
};

export type DetailCardMultiWorkstationFactoryDocumentOverrides = {
  planPrompt?: string;
  reviewPrompt?: string;
};

export function buildDetailCardFactoryDocumentQueryResult(
  data: CurrentFactoryDocument | undefined,
) {
  latestDetailCardCurrentFactoryDocument = data ?? null;

  return {
    data,
    error: null,
    failureCount: 0,
    failureReason: null,
    fetchStatus: "idle",
    isError: false,
    isFetched: true,
    isFetchedAfterMount: true,
    isFetching: false,
    isInitialLoading: false,
    isLoading: false,
    isLoadingError: false,
    isPaused: false,
    isPending: false,
    isPlaceholderData: false,
    isRefetchError: false,
    isRefetching: false,
    isStale: true,
    isSuccess: true,
    promise: Promise.resolve(data),
    refetch: async () => data as never,
    status: "success",
  } as never;
}

export function buildDetailCardFactoryDocumentSaveHookReturn(
  saveAsync: Mock<
    (input: FactoryDocumentSaveInput) => Promise<CurrentFactoryDocument>
  >,
  options?: { isPending?: boolean },
): MockFactoryDocumentSaveReturn {
  return mockFactoryDocumentSave({
    isPending: options?.isPending,
    saveAsync,
  });
}

export function createDetailCardDeferredFactoryDocumentSave() {
  return createDeferredPromise<CurrentFactoryDocument>();
}

export function buildDetailCardEditableFactoryDocument(
  overrides?: DetailCardEditableFactoryDocumentOverrides,
): CurrentFactoryDocument {
  const workerOptions = overrides?.workerOptions ?? ["reviewer", "planner"];

  return {
    name: "Current Factory",
    version: { ...DETAIL_CARD_SAVE_FACTORY_VERSION },
    workers: workerOptions.map((name, index) => ({
      model: overrides?.model ?? `gpt-5.${index + 5}`,
      modelProvider:
        overrides?.modelProvider ??
        (index === 0 ? ("CURSOR" as const) : ("CODEX" as const)),
      name,
      type: "MODEL_WORKER",
    })),
    workstations: [
      {
        behavior: overrides?.behavior ?? "STANDARD",
        body:
          overrides?.prompt ??
          "Review the latest story changes before approval.",
        id: "review",
        inputs: [{ state: "queued", workType: "story" }],
        name: "Review",
        outputs: [{ state: "approved", workType: "story" }],
        promptFile: "prompts/review.md",
        worker: overrides?.workerName ?? "reviewer",
      },
    ],
    workTypes: [],
  };
}

export function buildDetailCardMultiWorkstationFactoryDocument(
  overrides?: DetailCardMultiWorkstationFactoryDocumentOverrides,
): CurrentFactoryDocument {
  return {
    name: "Current Factory",
    version: { ...DETAIL_CARD_SAVE_FACTORY_VERSION },
    workers: [
      {
        model: "gpt-5.5",
        modelProvider: "CURSOR",
        name: "reviewer",
        type: "MODEL_WORKER",
      },
      {
        model: "gpt-5.6",
        modelProvider: "CODEX",
        name: "planner",
        type: "MODEL_WORKER",
      },
    ],
    workstations: [
      {
        body:
          overrides?.reviewPrompt ??
          "Review the latest story changes before approval.",
        id: "review",
        inputs: [{ state: "queued", workType: "story" }],
        name: "Review",
        outputs: [{ state: "approved", workType: "story" }],
        promptFile: "prompts/review.md",
        worker: "reviewer",
      },
      {
        body: overrides?.planPrompt ?? "Plan the implementation.",
        id: "plan",
        inputs: [{ state: "queued", workType: "story" }],
        name: "Plan",
        outputs: [{ state: "approved", workType: "story" }],
        promptFile: "prompts/plan.md",
        worker: "planner",
      },
    ],
    workTypes: [],
  };
}

export function buildDetailCardMultiResourceFactoryDocument(): CurrentFactoryDocument {
  return {
    name: "Current Factory",
    version: { ...DETAIL_CARD_SAVE_FACTORY_VERSION },
    resources: [
      {
        capacity: 2,
        name: "agent-slot",
        type: "INVOCATION_SLOT",
      },
      {
        capacity: 5,
        model: "gpt-audio",
        name: "voice-model",
        type: "MODEL",
      },
    ],
    workers: [
      {
        model: "gpt-5.5",
        modelProvider: "CURSOR",
        name: "reviewer",
        resources: [{ capacity: 1, name: "agent-slot" }],
        type: "MODEL_WORKER",
      },
    ],
    workstations: [
      {
        body: "Review the latest story changes before approval.",
        id: "review",
        inputs: [{ state: "queued", workType: "story" }],
        name: "Review",
        outputs: [{ state: "approved", workType: "story" }],
        promptFile: "prompts/review.md",
        resources: [{ capacity: 1, name: "agent-slot" }],
        worker: "reviewer",
      },
    ],
    workTypes: [],
  };
}

export function buildDetailCardSharedWorkerFactoryDocument(overrides?: {
  prompt?: string;
}): CurrentFactoryDocument {
  return {
    name: "Current Factory",
    version: { ...DETAIL_CARD_SAVE_FACTORY_VERSION },
    workers: [
      {
        model: "gpt-5.5",
        modelProvider: "CURSOR",
        name: "processor",
        type: "MODEL_WORKER",
      },
    ],
    workstations: [
      {
        body:
          overrides?.prompt ??
          "Review the latest story changes before approval.",
        id: "review",
        inputs: [{ state: "queued", workType: "story" }],
        name: "Review",
        outputs: [{ state: "approved", workType: "story" }],
        promptFile: "prompts/review.md",
        worker: "processor",
      },
      {
        body: "Plan the implementation.",
        id: "plan",
        inputs: [{ state: "queued", workType: "story" }],
        name: "Plan",
        outputs: [{ state: "approved", workType: "story" }],
        promptFile: "prompts/plan.md",
        worker: "processor",
      },
    ],
    workTypes: [],
  };
}

export function buildDetailCardCurrentSelection(
  overrides: Partial<CurrentSelectionState> = {},
): CurrentSelectionState {
  const currentFactoryDefinition =
    overrides.currentFactoryDefinition ??
    resolveLatestDetailCardCurrentFactoryDocument() ??
    buildDetailCardEditableFactoryDocument();
  const selectedWorkerName =
    overrides.selectedWorkerName ??
    (overrides.selection?.kind === "worker"
      ? overrides.selection.workerName
      : null);
  const selectedResourceName =
    overrides.selectedResourceName ??
    (overrides.selection?.kind === "resource"
      ? overrides.selection.resourceName
      : null);

  return {
    canRedoSelection: false,
    canUndoSelection: false,
    completedWorkItems: [],
    currentFactoryDefinition,
    failedWorkItems: [],
    openTerminalWorkDetail: () => undefined,
    redoSelection: () => undefined,
    selectedNode: null,
    selectedNodeActiveExecutions: [],
    selectedNodeProviderSessions: [],
    selectedNodeWorkstationRequests: [],
    selectedStateCurrentWorkItems: [],
    selectedStatePlace: null,
    selectedStateTerminalHistoryWorkItems: [],
    selectedStateTokenCount: 0,
    selectedWorkDispatchAttempts: [],
    selectedWorkID: null,
    selectedWorkOperationHistory: [],
    selectedWorkProviderSessions: [],
    selectedWorkRequestHistory: [],
    selectedWorkWorkstationRequests: [],
    selectedWorkstationRequest: null,
    selectedDocTargetPath: null,
    selectedWorker:
      currentFactoryDefinition.workers?.find(
        (worker) => worker.name === selectedWorkerName,
      ) ?? null,
    selectedResource:
      currentFactoryDefinition.resources?.find(
        (resource) => resource.name === selectedResourceName,
      ) ?? null,
    selectedResourceName,
    selectedResourceTokenCount: null,
    selectedWorkerName,
    selectedWorkerWorkstationNames: [],
    selectedWorkType: null,
    selectedWorkTypeName: null,
    selection: null,
    selectStateNode: () => undefined,
    selectStateWorkItem: () => undefined,
    selectWorkByID: () => undefined,
    selectWorkItem: () => undefined,
    clearSelectedDocIfMatching: () => undefined,
    clearSelectedFactoryGraphNodeIfMatching: () => undefined,
    clearSelectedStateNodeIfMatching: () => undefined,
    clearSelectedWorkerIfMatching: () => undefined,
    selectDoc: () => undefined,
    selectResource: () => undefined,
    selectWorker: () => undefined,
    selectWorkType: () => undefined,
    selectWorkstation: () => undefined,
    selectWorkstationRequest: () => undefined,
    terminalWorkDetail: null,
    undoSelection: () => undefined,
    ...overrides,
  };
}

export function buildDetailCardWorkStateFactoryDocument(
  overrides?: Partial<CurrentFactoryDocument>,
): CurrentFactoryDocument {
  return {
    name: "Current Factory",
    version: { ...DETAIL_CARD_SAVE_FACTORY_VERSION },
    workers: [],
    workstations: [],
    workTypes: [
      {
        name: "story",
        states: [
          { name: "implemented", type: "PROCESSING" },
          { name: "complete", type: "TERMINAL" },
          { name: "blocked", type: "FAILED" },
        ],
      },
    ],
    ...overrides,
  };
}

export function expandDetailCardResourceConfiguration(
  buttonName = "Expand resource configuration editor",
  headingName = "Resource configuration",
) {
  const section = screen
    .getAllByRole("heading", { name: headingName })
    .at(-1)
    ?.closest("section");
  if (!section) {
    throw new Error("expected editable resource configuration section");
  }

  const expandButton = within(section).queryByRole("button", {
    name: buttonName,
  });
  if (expandButton) {
    fireEvent.click(expandButton);
  }
}

export function currentSelectionHeaderActionSection() {
  const article = screen.getByRole("article", { name: "Current selection" });
  const section = article.querySelector("[data-action-row-section='actions']");
  if (!section) {
    throw new Error("expected current selection header action section");
  }

  return section as HTMLElement;
}

export function expandDetailCardWorkerConfiguration(
  buttonName = "Expand worker configuration editor",
  headingName = "Worker configuration",
) {
  const section = screen
    .getAllByRole("heading", { name: headingName })
    .at(-1)
    ?.closest("section");
  if (!section) {
    throw new Error("expected editable worker configuration section");
  }

  const expandButton = within(section).queryByRole("button", {
    name: buttonName,
  });
  if (expandButton) {
    fireEvent.click(expandButton);
  }
}

export function expandDetailCardWorkstationConfiguration(
  buttonName = "Expand editable configuration",
  headingName = "Configuration",
) {
  const section = screen
    .getAllByRole("heading", { name: headingName })
    .at(-1)
    ?.closest("section");
  if (!section) {
    throw new Error("expected editable configuration section");
  }

  fireEvent.click(
    within(section).getByRole("button", {
      name: buttonName,
    }),
  );
}

export function workstationSaveButtons() {
  return screen.getAllByRole("button", { name: "Save changes" });
}

export function workstationFooterSaveButton() {
  const footer = workstationSaveButtons().at(-1);
  if (!footer) {
    throw new Error("expected workstation Save changes button");
  }

  return footer;
}

export function clickWorkstationSave() {
  fireEvent.click(workstationFooterSaveButton());
}
