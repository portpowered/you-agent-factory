import type { EditableWorkstationBehavior } from "../../../current-factory-definition/lib/workstation-behavior";
import type {
  EditableWorkstationDraft,
  EditableWorkstationValues,
} from "../../../current-factory-definition/lib/workstation-editable-values";
import type { ApiRunnerID } from "../messages/runner-openapi-enums";
import {
  resolveDraftForBehaviorChange,
  updateEditableWorkstationCronDraft,
} from "./editable-workstation-cron-draft-mutators";

export type EditableWorkstationSessionDraftState = {
  draft: EditableWorkstationDraft;
  latestDefinitionDraft: EditableWorkstationDraft;
  sessionStartDraft: EditableWorkstationDraft;
};

function updateEditableWorkstationDraft<
  TSessionState extends EditableWorkstationSessionDraftState,
>(
  setSessionState: (
    updater: (currentState: TSessionState | null) => TSessionState | null,
  ) => void,
  updater: (draft: EditableWorkstationDraft) => EditableWorkstationDraft,
) {
  setSessionState((currentState) =>
    currentState
      ? {
          ...currentState,
          draft: updater(currentState.draft),
        }
      : currentState,
  );
}

export function buildEditableWorkstationConfigurationMutators<
  TSessionState extends EditableWorkstationSessionDraftState,
>({
  selectedEditableValues,
  setSessionState,
}: {
  selectedEditableValues: EditableWorkstationValues;
  setSessionState: (
    updater: (currentState: TSessionState | null) => TSessionState | null,
  ) => void;
}) {
  const updateDraft = (
    updater: (draft: EditableWorkstationDraft) => EditableWorkstationDraft,
  ) => updateEditableWorkstationDraft(setSessionState, updater);

  return {
    markChangesSaved: () => {
      setSessionState((currentState) =>
        currentState
          ? {
              ...currentState,
              latestDefinitionDraft: currentState.draft,
              sessionStartDraft: currentState.draft,
            }
          : currentState,
      );
    },
    onBehaviorChange: (value: EditableWorkstationBehavior) => {
      setSessionState((currentState) =>
        currentState
          ? {
              ...currentState,
              draft: resolveDraftForBehaviorChange(
                currentState.draft,
                value,
                selectedEditableValues,
              ),
            }
          : currentState,
      );
    },
    onCronExpiryWindowChange: (value: string) => {
      setSessionState((currentState) =>
        currentState
          ? {
              ...currentState,
              draft: updateEditableWorkstationCronDraft(currentState.draft, {
                expiryWindow: value,
              }),
            }
          : currentState,
      );
    },
    onCronJitterChange: (value: string) => {
      setSessionState((currentState) =>
        currentState
          ? {
              ...currentState,
              draft: updateEditableWorkstationCronDraft(currentState.draft, {
                jitter: value,
              }),
            }
          : currentState,
      );
    },
    onCronScheduleChange: (value: string) => {
      setSessionState((currentState) =>
        currentState
          ? {
              ...currentState,
              draft: updateEditableWorkstationCronDraft(currentState.draft, {
                schedule: value,
              }),
            }
          : currentState,
      );
    },
    onCronTriggerAtStartChange: (value: boolean) => {
      setSessionState((currentState) =>
        currentState
          ? {
              ...currentState,
              draft: updateEditableWorkstationCronDraft(currentState.draft, {
                triggerAtStart: value,
              }),
            }
          : currentState,
      );
    },
    onNameChange: (value: string) => {
      updateDraft((draft) => ({ ...draft, name: value }));
    },
    onPromptChange: (value: string) => {
      updateDraft((draft) => ({ ...draft, prompt: value }));
    },
    onResetToLatest: () => {
      setSessionState((currentState) =>
        currentState
          ? {
              ...currentState,
              draft: currentState.latestDefinitionDraft,
              sessionStartDraft: currentState.latestDefinitionDraft,
            }
          : currentState,
      );
    },
    onRunnerChange: (value: ApiRunnerID | null) => {
      updateDraft((draft) => ({ ...draft, runnerName: value }));
    },
    onWorkerChange: (value: string) => {
      updateDraft((draft) => ({ ...draft, workerName: value }));
    },
  };
}
