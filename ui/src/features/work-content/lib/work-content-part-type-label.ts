import type { components } from "../../../api/generated/openapi";
import {
  getWorkContentInspectMessages,
  type WorkContentPartTypeLabels,
} from "../messages/work-content";

export type WorkContentPartType = components["schemas"]["WorkContentPartType"];

const PART_TYPE_LABEL_KEYS: Record<string, keyof WorkContentPartTypeLabels> = {
  AUDIO: "audio",
  BINARY: "binary",
  IMAGE: "image",
  JSON: "json",
  TEXT: "text",
  audio: "audio",
  binary: "binary",
  image: "image",
  json: "json",
  text: "text",
};

export function workContentPartTypeLabel(
  type: string | undefined,
  labels = getWorkContentInspectMessages().partTypeLabels,
): string {
  if (!type) {
    return labels.fallback;
  }

  const labelKey = PART_TYPE_LABEL_KEYS[type];
  if (labelKey && labelKey !== "unknownType" && labelKey !== "fallback") {
    return labels[labelKey];
  }

  return labels.unknownType(type);
}
