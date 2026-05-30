import type { components } from "../../../api/generated/openapi";
import {
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../components/ui/dashboard-typography";
import { AuthoredBodyText, REQUEST_AUTHORED_TEXT_CLASS } from "../../../lib/authored-body-text";
import { cn } from "../../../lib/cn";
import { describeWorkContentPart } from "../lib/describe-work-content-part";

export type WorkContent = components["schemas"]["WorkContent"];
export type WorkContentPart = components["schemas"]["WorkContentPart"];

export function WorkContentPartList({
  content,
  listClassName,
}: {
  content: WorkContent;
  listClassName?: string;
}) {
  return (
    <div className={cn("grid gap-2", listClassName)}>
      {content.map((part, index) => renderWorkContentPart(part, index))}
    </div>
  );
}

function renderWorkContentPart(part: WorkContentPart, index: number) {
  if (part.type === "text" || part.type === "TEXT") {
    return typeof part.text === "string" ? (
      <AuthoredBodyText key={`work-content-part-${index}`} value={part.text} />
    ) : null;
  }

  if (part.type === "JSON") {
    const value =
      typeof part.json === "string"
        ? part.json
        : JSON.stringify(part.json ?? null, null, 2);
    return (
      <pre className={REQUEST_AUTHORED_TEXT_CLASS} key={`work-content-part-${index}`}>
        <code>{value}</code>
      </pre>
    );
  }

  return (
    <div
      className={cn(
        "rounded-lg border border-af-border bg-af-surface-raised p-3 text-af-text-muted",
        DASHBOARD_SUPPORTING_TEXT_CLASS,
      )}
      key={`work-content-part-${index}`}
    >
      {describeWorkContentPart(part)}
    </div>
  );
}
