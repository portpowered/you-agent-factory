import type { components } from "../../../api/generated/openapi";
import { workContentPartTypeLabel } from "./work-content-part-type-label";

export type WorkContentPart = components["schemas"]["WorkContentPart"];

function workContentURLDisplayName(url: string): string | undefined {
  try {
    const parsed = new URL(url);
    if (parsed.protocol === "file:") {
      const pathSegments = parsed.pathname.split("/").filter(Boolean);
      if (pathSegments.length > 0) {
        return pathSegments[pathSegments.length - 1];
      }
      if (parsed.hostname) {
        return parsed.hostname;
      }
    }
    if (parsed.protocol === "data:") {
      return "embedded data";
    }
    const pathSegments = parsed.pathname.split("/").filter(Boolean);
    if (pathSegments.length > 0) {
      return pathSegments[pathSegments.length - 1];
    }
    return parsed.hostname || undefined;
  } catch {
    return undefined;
  }
}

export function describeWorkContentPart(part: WorkContentPart): string {
  const file = "file" in part ? part.file : undefined;
  if (typeof file === "string" && file) {
    return `${workContentPartTypeLabel(part.type)}: ${file}`;
  }

  const url = "url" in part ? part.url : undefined;
  if (typeof url === "string" && url) {
    const displayName = workContentURLDisplayName(url);
    if (displayName) {
      return `${workContentPartTypeLabel(part.type)}: ${displayName}`;
    }
    return `${workContentPartTypeLabel(part.type)}: ${url}`;
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
