import { isPollerRunWorkstationType } from "../../../current-factory-definition/lib/worker-workstation-taxonomy";
import type { EditableWorkstationBehavior } from "../../../current-factory-definition/lib/workstation-behavior";
import {
  createEmptyEditableWorkstationCronDraft,
  type EditableWorkstationCronDraft,
  type EditableWorkstationDraft,
  type EditableWorkstationValues,
} from "../../../current-factory-definition/lib/workstation-editable-values";

export function resolveDraftForBehaviorChange(
  draft: EditableWorkstationDraft,
  behavior: EditableWorkstationBehavior,
  selectedEditableValues: EditableWorkstationValues,
): EditableWorkstationDraft {
  if (isPollerRunWorkstationType(draft.workstationType)) {
    return {
      ...draft,
      behavior: "POLLER",
      cron: null,
      prompt: "",
      runnerName: null,
    };
  }

  if (behavior === "CRON") {
    return {
      ...draft,
      behavior,
      cron:
        draft.cron ??
        (selectedEditableValues.cron
          ? { ...selectedEditableValues.cron }
          : createEmptyEditableWorkstationCronDraft()),
    };
  }

  return {
    ...draft,
    behavior,
    cron: null,
  };
}

export function updateEditableWorkstationCronDraft(
  draft: EditableWorkstationDraft,
  cronPatch: Partial<EditableWorkstationCronDraft>,
): EditableWorkstationDraft {
  if (draft.behavior !== "CRON") {
    return draft;
  }

  const cron = draft.cron ?? createEmptyEditableWorkstationCronDraft();
  return {
    ...draft,
    cron: {
      ...cron,
      ...cronPatch,
    },
  };
}
