import { Label } from "@you-agent-factory/components/primitives";

import type { WorkerTimelineContentBlock } from "../lib/worker-session-timeline-projection-types";
import type { WorkerSessionTimelineMessages } from "../messages/worker-session-timeline";
import {
  BoundedCode,
  BoundedText,
} from "./worker-session-timeline-detail-primitives";

export function WorkerSessionTimelineContentBlockDetails({
  block,
  messages,
}: {
  block: WorkerTimelineContentBlock;
  messages: WorkerSessionTimelineMessages;
}) {
  return (
    <section
      aria-label={messages.messageTextLabel}
      className="grid min-w-0 gap-2"
    >
      <Label>{block.kind}</Label>
      {block.text ? (
        <BoundedText
          collapseLabel={messages.collapseContentAction}
          expandLabel={messages.expandContentAction}
          label={messages.messageTextLabel}
          value={block.text}
        />
      ) : null}
      {block.argumentsSummary !== undefined ? (
        <BoundedCode
          collapseLabel={messages.collapseContentAction}
          expandLabel={messages.expandContentAction}
          label={messages.toolArgumentsLabel}
          value={block.argumentsSummary}
        />
      ) : null}
      {block.structuredOutput !== undefined ? (
        <BoundedCode
          collapseLabel={messages.collapseContentAction}
          expandLabel={messages.expandContentAction}
          label={messages.toolResultLabel}
          value={block.structuredOutput}
        />
      ) : null}
    </section>
  );
}
