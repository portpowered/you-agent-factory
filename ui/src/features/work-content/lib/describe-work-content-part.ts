import type { components } from "../../../api/generated/openapi";
import { workContentPartTypeLabel } from "./work-content-part-type-label";

export type WorkContentPart = components["schemas"]["WorkContentPart"];

export function describeWorkContentPart(part: WorkContentPart): string {
  const file = "file" in part ? part.file : undefined;
  if (typeof file === "string" && file) {
    return `${workContentPartTypeLabel(part.type)}: ${file}`;
  }

  const label = "label" in part ? part.label : undefined;
  if (typeof label === "string" && label) {
    return `${workContentPartTypeLabel(part.type)}: ${label}`;
  }

  const contentType = "contentType" in part ? part.contentType : undefined;
  if (typeof contentType === "string" && contentType) {
    return `${workContentPartTypeLabel(part.type)} (${contentType})`;
  }

  return workContentPartTypeLabel(part.type);
}
