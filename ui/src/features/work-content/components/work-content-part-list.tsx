import type { components } from "../../../api/generated/openapi";
import { DashboardText, SurfacePanel } from "../../../components/ui";
import {
  AuthoredBodyText,
  AUTHORED_BODY_TEXT_CLASS,
} from "../../../lib/authored-body-text";
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
      <pre
        className={AUTHORED_BODY_TEXT_CLASS}
        key={`work-content-part-${index}`}
      >
        <code>{value}</code>
      </pre>
    );
  }

  return (
    <SurfacePanel asChild key={`work-content-part-${index}`} radius="lg">
      <DashboardText className="text-on-surface-variant" variant="supporting">
        {describeWorkContentPart(part)}
      </DashboardText>
    </SurfacePanel>
  );
}
