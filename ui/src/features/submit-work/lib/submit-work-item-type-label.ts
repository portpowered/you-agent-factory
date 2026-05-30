import {
  getWorkContentInspectMessages,
  type WorkContentPartTypeLabels,
  workContentPartTypeLabel,
} from "../../work-content/public";
import type { SubmitWorkDraftItemType } from "../components/submit-work-card";
import {
  getSubmitWorkMessages,
  type SubmitWorkItemTypeLabelKey,
} from "../messages/submit-work";

const SUBMIT_ONLY_PART_TYPES = new Set<SubmitWorkItemTypeLabelKey>([
  "document",
  "video",
]);

function submitWorkItemRowTypeLabels(
  locale?: string | null,
): WorkContentPartTypeLabels {
  const resolvedLocale = locale ?? undefined;
  const inspectLabels =
    getWorkContentInspectMessages(resolvedLocale).partTypeLabels;
  const submitMessages = getSubmitWorkMessages(locale);

  return {
    ...inspectLabels,
    audio: submitMessages.addItemOptionLabel("audio"),
    image: submitMessages.addItemOptionLabel("image"),
    text: submitMessages.addItemOptionLabel("text"),
    unknownType: (type) => {
      if (SUBMIT_ONLY_PART_TYPES.has(type as SubmitWorkItemTypeLabelKey)) {
        return submitMessages.addItemOptionLabel(
          type as SubmitWorkItemTypeLabelKey,
        );
      }
      return inspectLabels.unknownType(type);
    },
  };
}

export function submitWorkItemRowTypeLabel(
  itemType: SubmitWorkDraftItemType,
  locale?: string | null,
): string {
  return workContentPartTypeLabel(
    itemType,
    submitWorkItemRowTypeLabels(locale),
  );
}
