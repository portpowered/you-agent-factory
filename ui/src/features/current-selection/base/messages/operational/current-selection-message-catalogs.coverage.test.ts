import { describe, expect, it } from "vitest";

import { SUPPORTED_LOCALES } from "../../../../../i18n";
import {
  getWorkstationDetailMessages,
  type WorkstationDetailMessages,
} from "../../../messages/workstation-detail";
import {
  getResourceDetailMessages,
  type ResourceDetailMessages,
} from "../../../resource-selection/messages/resource-detail";
import {
  getWorkTypeDetailMessages,
  type WorkTypeDetailMessages,
} from "../../../work-type-selection/messages/work-type-detail";
import {
  getWorkerDetailMessages,
  type WorkerDetailMessages,
} from "../../../worker-selection/messages/worker-detail";
import {
  type CurrentSelectionDetailMessages,
  getCurrentSelectionDetailMessages,
} from "../shell/current-selection-detail";
import {
  type CurrentSelectionDispatchHistoryMessages,
  getCurrentSelectionDispatchHistoryMessages,
} from "../shell/current-selection-dispatch-history";
import {
  type EditableConfigurationControlsMessages,
  getEditableConfigurationControlsMessages,
} from "./editable-configuration-controls";

const assertResolvedValue = (value: unknown) => {
  expect(typeof value).toBe("string");
  expect((value as string).length).toBeGreaterThan(0);
};

const assertCatalogValuesResolve = (
  catalog: Record<string, unknown>,
  invoke: (key: string, formatter: (...args: never[]) => unknown) => unknown[],
) => {
  for (const [key, value] of Object.entries(catalog)) {
    if (typeof value === "function") {
      for (const rendered of invoke(
        key,
        value as (...args: never[]) => unknown,
      )) {
        assertResolvedValue(rendered);
      }
      continue;
    }

    assertResolvedValue(value);
  }
};

const invokeCurrentSelectionDetail = (
  key: string,
  formatter: (...args: never[]) => unknown,
) => {
  switch (key satisfies keyof CurrentSelectionDetailMessages) {
    case "attemptAriaLabel":
    case "collapseAttemptAction":
    case "expandAttemptAction":
    case "attemptTitle":
      return [formatter(2 as never)];
    case "collapseRequestBodyAction":
    case "collapseResponseBodyAction":
    case "expandRequestBodyAction":
    case "expandResponseBodyAction":
      return [formatter()];
    case "selectWorkItemLabel":
    case "openWorkItemAction":
      return [formatter("Review Story" as never)];
    case "terminalOutcomeLabel":
      return [
        formatter("ACCEPTED" as never),
        formatter("CONTINUE" as never),
        formatter("FAILED" as never),
        formatter("REJECTED" as never),
        formatter("CUSTOM_OUTCOME" as never),
      ];
    case "terminalRequestContext":
      return [
        formatter({
          outcome: "Accepted",
          workstation: "Review",
        } as never),
        formatter({
          outcome: "Failed",
          providerSession: "session-123",
          workstation: "Review",
        } as never),
      ];
    default:
      throw new Error(`Unhandled current-selection detail formatter ${key}`);
  }
};

const invokeDispatchHistory = (
  key: string,
  formatter: (...args: never[]) => unknown,
) => {
  switch (key satisfies keyof CurrentSelectionDispatchHistoryMessages) {
    case "dispatchHistoryCountLabel":
    case "inferenceAttemptAccessibleLabel":
    case "inferenceAttemptLabel":
      return [formatter(1 as never), formatter(3 as never)];
    case "requestAttemptTitle":
    case "responseAttemptTitle":
      return [formatter(undefined as never), formatter(2 as never)];
    case "requestAttemptLabel":
    case "responseAttemptLabel":
      return [formatter("pending" as never)];
    case "selectWorkItemAccessibleLabel":
    case "openWorkItemActionLabel":
      return [formatter("Review Story" as never)];
    case "formatMoveTransition":
      return [formatter("init" as never, "review" as never)];
    case "localizeMoveSource":
      return [
        formatter("api" as never),
        formatter("cli" as never),
        formatter("cascading-failure" as never),
        formatter("future-source" as never),
      ];
    case "logicalMoveDispatchRowAccessibleLabel":
    case "workstationDispatchRowAccessibleLabel":
      return [
        formatter("Review" as never, "dispatch-1" as never),
        formatter(undefined as never, undefined as never),
      ];
    case "operatorMoveRowAccessibleLabel":
      return [formatter("init → review" as never)];
    default:
      throw new Error(`Unhandled dispatch-history formatter ${key}`);
  }
};

const invokeResourceDetail = (
  key: string,
  formatter: (...args: never[]) => unknown,
) => {
  switch (key satisfies keyof ResourceDetailMessages) {
    case "editableConfigurationNameDuplicate":
      return [formatter("duplicate-resource" as never)];
    case "editableConfigurationOverwriteWarning":
      return [formatter("Capacity" as never)];
    case "editableConfigurationSaveSuccess":
      return [formatter("agent-slot" as never)];
    case "editableConfigurationSharedImpactWarning":
      return [
        formatter(
          "agent-slot" as never,
          "reviewer" as never,
          "Review" as never,
        ),
      ];
    case "localizeResourceType":
      return [
        formatter("INVOCATION_SLOT" as never),
        formatter("MODEL" as never),
        formatter("future-type" as never),
      ];
    default:
      throw new Error(`Unhandled resource-detail formatter ${key}`);
  }
};

const invokeWorkerDetail = (
  key: string,
  formatter: (...args: never[]) => unknown,
) => {
  switch (key satisfies keyof WorkerDetailMessages) {
    case "editableConfigurationOverwriteWarning":
      return [formatter("Model provider" as never)];
    case "editableConfigurationNameDuplicate":
      return [formatter("duplicate-worker" as never)];
    case "editableConfigurationSaveSuccess":
      return [formatter("reviewer" as never)];
    case "editableConfigurationSharedImpactWarning":
      return [formatter("reviewer" as never, "Review, Plan" as never)];
    case "editableConfigurationTimeoutInvalid":
      return [formatter("0" as never), formatter("bad" as never)];
    case "localizeExecutorProvider":
      return [
        formatter("SCRIPT_WRAP" as never),
        formatter("future-executor" as never),
      ];
    case "localizeModelLocality":
      return [
        formatter("CLOUD" as never),
        formatter("LOCAL" as never),
        formatter("future-locality" as never),
      ];
    case "localizeModelProvider":
      return [
        formatter("CODEX" as never),
        formatter("CODEX" as never),
        formatter("future-provider" as never),
      ];
    case "localizeTimeoutUnit":
      return [
        formatter("s" as never),
        formatter("m" as never),
        formatter("h" as never),
        formatter("future-unit" as never),
      ];
    case "localizeWorkerType":
      return [
        formatter("MODEL_WORKER" as never),
        formatter("SCRIPT_WORKER" as never),
        formatter("HOSTED_WORKER" as never),
        formatter("future-type" as never),
      ];
    default:
      throw new Error(`Unhandled worker-detail formatter ${key}`);
  }
};

const invokeWorkTypeDetail = (
  key: string,
  formatter: (...args: never[]) => unknown,
) => {
  switch (key satisfies keyof WorkTypeDetailMessages) {
    case "editableConfigurationNameDuplicate":
    case "editableConfigurationSaveSuccess":
      return [formatter("story" as never)];
    case "localizeWorkStateType":
      return [
        formatter("INITIAL" as never),
        formatter("PROCESSING" as never),
        formatter("TERMINAL" as never),
        formatter("FAILED" as never),
      ];
    case "selectWorkStateGraphNodeLabel":
      return [formatter("queued" as never)];
    default:
      throw new Error(`Unhandled work-type-detail formatter ${key}`);
  }
};

const invokeEditableConfigurationControls = (
  key: string,
  _formatter: (...args: never[]) => unknown,
) => {
  switch (key satisfies keyof EditableConfigurationControlsMessages) {
    default:
      throw new Error(
        `Unhandled editable-configuration-controls formatter ${key}`,
      );
  }
};

const invokeWorkstationDetailSecondaryFormatters = (
  key: string,
  formatter: (...args: never[]) => unknown,
): unknown[] | null => {
  switch (key) {
    case "localizeRunnerSelectionSource":
      return [
        formatter("factory" as never),
        formatter("legacy_provider" as never),
        formatter("future-source" as never),
      ];
    case "runnerFieldHelp":
      return [formatter("Gemini" as never, "Factory" as never)];
    case "historyRequestCountLabel":
    case "historyRunCountLabel":
      return [formatter(1 as never), formatter(3 as never)];
    case "editableConfigurationPromptAutocompleteSummary":
      return [
        formatter(1 as never, 1 as never),
        formatter(3 as never, 2 as never),
      ];
    case "editableConfigurationCronExpiryWindowInvalid":
    case "editableConfigurationCronJitterInvalid":
      return [formatter("0s" as never), formatter("bad" as never)];
    case "editableConfigurationCronScheduleInvalid":
      return [
        formatter("0 * * * *" as never, "bad field" as never),
        formatter("@every bad" as never, "invalid @every duration" as never),
      ];
    case "editableConfigurationVisitCountWorkstationInvalid":
    case "editableConfigurationInputGuardMatchInputInvalid":
    case "editableConfigurationInputGuardParentInputInvalid":
      return [formatter("Review" as never)];
    case "editableConfigurationInputGuardSpawnedByInvalid":
      return [formatter("Plan" as never)];
    case "localizeInputGuardType":
    case "localizeWorkstationGuardType":
      return [
        formatter("VISIT_COUNT" as never),
        formatter("SAME_NAME" as never),
        formatter("future-guard" as never),
      ];
    case "workstationInputSlotHeading":
      return [formatter("story" as never, "queued" as never)];
    case "openNamedWorkItemAction":
    case "selectWorkItemLabel":
      return [formatter("Review Story" as never)];
    case "providerSummary":
      return [
        formatter("codex" as never, null as never),
        formatter("codex" as never, "gpt-5.4" as never),
      ];
    case "requestDetailsUnavailable":
    case "requestStatusStartedAgo":
    case "scriptCommandSummary":
    case "selectWorkstationRequestLabel":
    case "selectedRequestLabel":
    case "workDetailsUnavailable":
      return [formatter("dispatch-review" as never)];
    case "selectProviderSessionLabel":
    case "selectRequestLabel":
      return [formatter("Review Story" as never, "dispatch-review" as never)];
    default:
      return null;
  }
};

const invokeWorkstationDetail = (
  key: string,
  formatter: (...args: never[]) => unknown,
) => {
  const secondaryResult = invokeWorkstationDetailSecondaryFormatters(
    key,
    formatter,
  );
  if (secondaryResult) {
    return secondaryResult;
  }

  switch (key satisfies keyof WorkstationDetailMessages) {
    case "editableConfigurationOverwriteWarning":
    case "editableConfigurationSaveConflictConfirmationDescription":
    case "runnerInheritanceFactoryLabel":
      return [formatter("prompt" as never)];
    case "editableConfigurationNameDuplicate":
      return [formatter("duplicate-workstation" as never)];
    case "editableConfigurationSaveSuccess":
      return [formatter("Review" as never)];
    case "editableConfigurationSharedWorkerScopeHint":
      return [formatter("Worker A" as never, "Review, Implement" as never)];
    case "localizeWorkstationBehavior":
      return [
        formatter("STANDARD" as never),
        formatter("POLLER" as never),
        formatter("FUTURE_BEHAVIOR" as never),
      ];
    case "localizeProviderSessionKind":
      return [
        formatter("session_id" as never),
        formatter("path" as never),
        formatter("future-kind" as never),
      ];
    case "localizeWorkstationKind":
      return [
        formatter("STANDARD" as never),
        formatter("POLLER" as never),
        formatter("future-kind" as never),
      ];
    case "localizeWorkstationType":
      return [
        formatter("MODEL_WORKSTATION" as never),
        formatter("MODEL_INVOKE" as never),
        formatter("LOGICAL_MOVE" as never),
        formatter("FUTURE_TYPE" as never),
      ];
    case "editableConfigurationModelInvokeBindingDuplicate":
    case "editableConfigurationModelInvokeBindingRequired":
      return [formatter("prompt" as never)];
    case "modelInvokeBindingSlotHeading":
      return [
        formatter("prompt" as never, "required" as never),
        formatter("voice" as never, "optional" as never),
      ];
    default:
      throw new Error(`Unhandled workstation-detail formatter ${key}`);
  }
};

describe("current-selection message catalogs", () => {
  it.each(SUPPORTED_LOCALES)(
    "resolves every %s current-selection detail value",
    (locale) => {
      assertCatalogValuesResolve(
        getCurrentSelectionDetailMessages(locale) as unknown as Record<
          string,
          unknown
        >,
        invokeCurrentSelectionDetail,
      );
    },
  );

  it.each(SUPPORTED_LOCALES)(
    "resolves every %s dispatch-history value",
    (locale) => {
      assertCatalogValuesResolve(
        getCurrentSelectionDispatchHistoryMessages(locale) as unknown as Record<
          string,
          unknown
        >,
        invokeDispatchHistory,
      );
    },
  );

  it.each(SUPPORTED_LOCALES)(
    "resolves every %s workstation-detail value",
    (locale) => {
      assertCatalogValuesResolve(
        getWorkstationDetailMessages(locale) as unknown as Record<
          string,
          unknown
        >,
        invokeWorkstationDetail,
      );
    },
  );

  it.each(SUPPORTED_LOCALES)(
    "resolves every %s worker-detail value",
    (locale) => {
      assertCatalogValuesResolve(
        getWorkerDetailMessages(locale) as unknown as Record<string, unknown>,
        invokeWorkerDetail,
      );
    },
  );

  it.each(SUPPORTED_LOCALES)(
    "resolves every %s resource-detail value",
    (locale) => {
      assertCatalogValuesResolve(
        getResourceDetailMessages(locale) as unknown as Record<string, unknown>,
        invokeResourceDetail,
      );
    },
  );

  it.each(SUPPORTED_LOCALES)(
    "resolves every %s editable-configuration-controls value",
    (locale) => {
      assertCatalogValuesResolve(
        getEditableConfigurationControlsMessages(locale) as unknown as Record<
          string,
          unknown
        >,
        invokeEditableConfigurationControls,
      );
    },
  );

  it.each(SUPPORTED_LOCALES)(
    "resolves every %s work-type-detail value",
    (locale) => {
      assertCatalogValuesResolve(
        getWorkTypeDetailMessages(locale) as unknown as Record<string, unknown>,
        invokeWorkTypeDetail,
      );
    },
  );
});
