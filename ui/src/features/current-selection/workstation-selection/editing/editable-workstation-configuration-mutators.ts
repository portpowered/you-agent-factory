import type { EditableWorkstationBehavior } from "../../../current-factory-definition/lib/workstation-behavior";
import type {
  EditableWorkstationDraft,
  EditableWorkstationValues,
} from "../../../current-factory-definition/lib/workstation-editable-values";
import {
  resolveDraftForBehaviorChange,
  updateEditableWorkstationCronDraft,
} from "./editable-workstation-cron-draft-mutators";
import type { RunnerID } from "./runner-metadata";

export type EditableWorkstationSessionDraftState = {
  draft: EditableWorkstationDraft;
  latestDefinitionDraft: EditableWorkstationDraft;
  sessionStartDraft: EditableWorkstationDraft;
};

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
    onPromptChange: (value: string) => {
      setSessionState((currentState) =>
        currentState
          ? {
              ...currentState,
              draft: {
                ...currentState.draft,
                prompt: value,
              },
            }
          : currentState,
      );
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
    onRunnerChange: (value: RunnerID | null) => {
      setSessionState((currentState) =>
        currentState
          ? {
              ...currentState,
              draft: {
                ...currentState.draft,
                runnerName: value,
              },
            }
          : currentState,
      );
    },
    onWorkerChange: (value: string) => {
      setSessionState((currentState) =>
        currentState
          ? {
              ...currentState,
              draft: {
                ...currentState.draft,
                workerName: value,
              },
            }
          : currentState,
      );
    },
  };
}
