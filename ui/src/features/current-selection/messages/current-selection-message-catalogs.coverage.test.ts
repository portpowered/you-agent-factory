import { SUPPORTED_LOCALES } from "../../../i18n";
import {
  getCurrentSelectionDetailMessages,
  type CurrentSelectionDetailMessages,
} from "./current-selection-detail";
import {
  getCurrentSelectionDispatchHistoryMessages,
  type CurrentSelectionDispatchHistoryMessages,
} from "./current-selection-dispatch-history";
import { getProviderSessionDetailMessages } from "./provider-session-detail";
import type { ProviderSessionDetailMessages } from "./provider-session-detail-types";
import {
  getWorkstationDetailMessages,
  type WorkstationDetailMessages,
} from "./workstation-detail";

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
    case "attemptTitle":
      return [formatter(2 as never)];
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
    case "relationshipLaneAriaLabel":
    case "relatedWorkSelectLabel":
    case "selectWorkItemAccessibleLabel":
    case "openWorkItemActionLabel":
      return [formatter("Review Story" as never)];
    case "relationshipStateLabel":
      return [formatter("Dependency" as never, "accepted" as never)];
    default:
      throw new Error(`Unhandled dispatch-history formatter ${key}`);
  }
};

const invokeProviderSessionDetail = (
  key: string,
  formatter: (...args: never[]) => unknown,
) => {
  switch (key satisfies keyof ProviderSessionDetailMessages) {
    case "lineLabel":
    case "unknownEventOnLineLabel":
      return [formatter({ lineNumber: 42 } as never)];
    case "orderLabel":
      return [
        formatter({ order: 1 } as never),
        formatter({ order: 2, turnIndex: 7 } as never),
      ];
    case "turnLabel":
      return [formatter({ index: 3 } as never)];
    default:
      throw new Error(`Unhandled provider-session formatter ${key}`);
  }
};

const invokeWorkstationDetail = (
  key: string,
  formatter: (...args: never[]) => unknown,
) => {
  switch (key satisfies keyof WorkstationDetailMessages) {
    case "editableConfigurationOverwriteWarning":
    case "editableConfigurationSaveConflictConfirmationDescription":
      return [formatter("prompt" as never)];
    case "historyRequestCountLabel":
    case "historyRunCountLabel":
      return [formatter(1 as never), formatter(3 as never)];
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
      throw new Error(`Unhandled workstation-detail formatter ${key}`);
  }
};

describe("current-selection message catalogs", () => {
  it.each(
    SUPPORTED_LOCALES,
  )("resolves every %s current-selection detail value", (locale) => {
    assertCatalogValuesResolve(
      getCurrentSelectionDetailMessages(locale) as unknown as Record<
        string,
        unknown
      >,
      invokeCurrentSelectionDetail,
    );
  });

  it.each(
    SUPPORTED_LOCALES,
  )("resolves every %s dispatch-history value", (locale) => {
    assertCatalogValuesResolve(
      getCurrentSelectionDispatchHistoryMessages(locale) as unknown as Record<
        string,
        unknown
      >,
      invokeDispatchHistory,
    );
  });

  it.each(
    SUPPORTED_LOCALES,
  )("resolves every %s provider-session detail value", (locale) => {
    assertCatalogValuesResolve(
      getProviderSessionDetailMessages(locale) as unknown as Record<
        string,
        unknown
      >,
      invokeProviderSessionDetail,
    );
  });

  it.each(
    SUPPORTED_LOCALES,
  )("resolves every %s workstation-detail value", (locale) => {
    assertCatalogValuesResolve(
      getWorkstationDetailMessages(locale) as unknown as Record<
        string,
        unknown
      >,
      invokeWorkstationDetail,
    );
  });
});
