import { Fragment } from "react";
import type { ReactNode } from "react";
import type { DashboardPlaceRef } from "../../api/dashboard/types";
import { cx } from "../../components/dashboard/classnames";
import {
  DASHBOARD_BODY_CODE_CLASS,
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_CODE_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../components/dashboard/typography";
import { DETAIL_COPY_CLASS } from "../../components/dashboard/widget-board";
import type { ExecutionDetailValue, ModelDetailValue } from "../../state/executionDetails";
import type {
  InferenceAttemptDetailProps,
  InferenceAttemptTextSectionProps,
  MetadataSectionProps,
  RequestCountSectionProps,
} from "./detail-card-types";

export const EXECUTION_PILL_CLASS = cx(
  "inline-flex rounded-full bg-af-info/15 px-2 py-[0.18rem] text-af-info-ink",
  DASHBOARD_SUPPORTING_CODE_CLASS,
);
export const PROVIDER_SESSION_CARD_CLASS =
  "rounded-lg border border-af-overlay/8 bg-af-overlay/4 p-[0.85rem]";
export const HISTORY_HEADER_CLASS =
  "flex items-center justify-between gap-3 rounded-lg border border-af-overlay/8 bg-af-overlay/4 px-3 py-2 [&_h4]:m-0";
export const HISTORY_TOGGLE_CLASS = cx(
  "shrink-0 cursor-pointer rounded-lg border border-af-accent/35 bg-af-accent/10 px-[0.65rem] py-[0.45rem] text-af-accent",
  DASHBOARD_SUPPORTING_TEXT_CLASS,
);
export const WORKSTATION_SUMMARY_ITEM_CLASS =
  "grid min-w-0 gap-[0.18rem] rounded-lg border border-af-overlay/8 bg-af-overlay/4 px-3 py-2";
export const INFERENCE_ATTEMPT_CARD_CLASS =
  "grid min-w-0 gap-[0.65rem] rounded-lg border border-af-overlay/8 p-[0.85rem]";
export const INFERENCE_ATTEMPT_DETAIL_CLASS = cx(
  "m-0 grid gap-[0.35rem] [&_dd]:m-0 [&_div]:grid [&_div]:min-w-0 [&_div]:grid-cols-[8.5rem_minmax(0,1fr)] [&_div]:gap-2",
  DASHBOARD_BODY_TEXT_CLASS,
);
export const INFERENCE_ATTEMPT_TEXT_CLASS = cx(
  "m-0 min-h-[20rem] whitespace-pre-wrap rounded-lg border border-af-overlay/8 bg-af-overlay/6 p-2 md:min-h-[26rem] lg:min-h-[min(70vh,36rem)] [overflow-wrap:anywhere]",
  DASHBOARD_BODY_CODE_CLASS,
);
export const REQUEST_AUTHORED_TEXT_CLASS = cx(
  "grid gap-3 rounded-lg border border-af-overlay/8 bg-af-overlay/6 p-3 [overflow-wrap:anywhere] [&_code]:rounded-[0.3rem] [&_code]:bg-af-overlay/12 [&_code]:px-[0.3rem] [&_code]:py-[0.15rem] [&_h1]:text-xl [&_h1]:font-semibold [&_h2]:text-lg [&_h2]:font-semibold [&_h3]:text-base [&_h3]:font-semibold [&_ol]:m-0 [&_ol]:list-decimal [&_ol]:pl-5 [&_pre]:m-0 [&_pre]:overflow-x-auto [&_pre]:rounded-lg [&_pre]:border [&_pre]:border-af-overlay/8 [&_pre]:bg-af-overlay/12 [&_pre]:p-3 [&_pre_code]:bg-transparent [&_pre_code]:p-0 [&_ul]:m-0 [&_ul]:list-disc [&_ul]:pl-5",
  DASHBOARD_BODY_TEXT_CLASS,
);
export const INFERENCE_REQUEST_PROMPT_LABEL = "Request prompt";
export const INFERENCE_RESPONSE_LABEL = "Response";
export const WORKSTATION_RESPONSE_TEXT_LABEL = "Response text";
export const RUNTIME_DETAILS_SECTION_CLASS =
  "mt-4 grid gap-[0.75rem] border-t border-af-overlay/8 pt-4 [&_h4]:m-0";
export const RUNTIME_DETAIL_VALUE_CLASS = "min-w-0 [overflow-wrap:anywhere]";
export const RUNTIME_DETAIL_CODE_CLASS = cx(
  DASHBOARD_BODY_CODE_CLASS,
  "[overflow-wrap:anywhere]",
);
export const TRACE_ACTION_LINK_CLASS =
  "inline-flex w-fit rounded-lg border border-af-accent/35 bg-af-accent/10 px-3 py-2 text-sm font-bold text-af-accent outline-af-accent transition hover:bg-af-accent/15 focus-visible:outline-2 focus-visible:outline-offset-2";
export const REQUEST_SELECTION_STATUS_CLASS = cx(
  "m-0 text-af-ink/68",
  DASHBOARD_SUPPORTING_TEXT_CLASS,
);
export const WORK_SELECTION_BUTTON_CLASS =
  "inline-flex w-fit rounded-lg border border-af-accent/35 bg-af-accent/10 px-[0.65rem] py-[0.45rem] text-xs font-bold text-af-accent outline-af-accent transition hover:bg-af-accent/15 focus-visible:outline-2 focus-visible:outline-offset-2";
export const REQUEST_HISTORY_TEXT_CLASS = cx(
  "m-0 whitespace-pre-wrap rounded-lg border border-af-overlay/8 bg-af-overlay/6 p-2 [overflow-wrap:anywhere]",
  DASHBOARD_BODY_CODE_CLASS,
);

const NO_CURRENT_WORK_IN_PLACE_COPY = "No current work is occupying this place.";
const NO_WORK_RECORDED_AT_SELECTED_TICK_COPY =
  "No work is recorded for this place at the selected tick.";
const SELECTED_TICK_WORK_UNAVAILABLE_COPY =
  "Represented work is unavailable for this place at the selected tick.";

interface RequestAuthoredHeadingBlock {
  level: 1 | 2 | 3 | 4 | 5 | 6;
  text: string;
  type: "heading";
}

interface RequestAuthoredListBlock {
  items: string[];
  type: "ordered-list" | "unordered-list";
}

interface RequestAuthoredParagraphBlock {
  text: string;
  type: "paragraph";
}

interface RequestAuthoredCodeBlock {
  code: string;
  language?: string;
  type: "code-block";
}

type RequestAuthoredBlock =
  | RequestAuthoredCodeBlock
  | RequestAuthoredHeadingBlock
  | RequestAuthoredListBlock
  | RequestAuthoredParagraphBlock;

export function InferenceAttemptTextSection({
  label,
  value,
}: InferenceAttemptTextSectionProps) {
  return (
    <section aria-label={label} className="grid gap-[0.3rem]">
      <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>{label}</span>
      <pre className={INFERENCE_ATTEMPT_TEXT_CLASS}>{value}</pre>
    </section>
  );
}

export function InferenceAttemptDetail({
  code = false,
  label,
  value,
}: InferenceAttemptDetailProps) {
  if (value === undefined || value === "") {
    return null;
  }

  return (
    <div>
      <dt>{label}</dt>
      <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
        {code ? <code className={RUNTIME_DETAIL_CODE_CLASS}>{value}</code> : value}
      </dd>
    </div>
  );
}

export function formatExecutionDetailValue(
  detail: ExecutionDetailValue | ModelDetailValue,
  label: "Model" | "Provider" | "Provider session",
) {
  if (detail.status === "available") {
    return <code className={RUNTIME_DETAIL_CODE_CLASS}>{detail.value}</code>;
  }

  const suffix = detail.status === "pending" ? " yet." : ".";
  return `${label} details are not available for this selected run${suffix}`;
}

export function RequestCountSection({ request }: RequestCountSectionProps) {
  return (
    <section aria-label="Request counts" className={RUNTIME_DETAILS_SECTION_CLASS}>
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>Request counts</h4>
      <dl className={INFERENCE_ATTEMPT_DETAIL_CLASS}>
        <InferenceAttemptDetail label="dispatchedCount" value={request.dispatched_request_count} />
        <InferenceAttemptDetail label="respondedCount" value={request.responded_request_count} />
        <InferenceAttemptDetail label="erroredCount" value={request.errored_request_count} />
      </dl>
    </section>
  );
}

export function MetadataSection({
  emptyMessage,
  metadata,
  title,
}: MetadataSectionProps) {
  const entries = Object.entries(metadata ?? {}).sort(([left], [right]) =>
    left.localeCompare(right),
  );

  return (
    <section aria-label={title} className={RUNTIME_DETAILS_SECTION_CLASS}>
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>{title}</h4>
      {entries.length > 0 ? (
        <dl>
          {entries.map(([key, value]) => (
            <div key={key}>
              <dt>{key}</dt>
              <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
                <code className={RUNTIME_DETAIL_CODE_CLASS}>{value}</code>
              </dd>
            </div>
          ))}
        </dl>
      ) : (
        <p className={DETAIL_COPY_CLASS}>{emptyMessage}</p>
      )}
    </section>
  );
}

export function isTerminalOrFailedPlace(place: DashboardPlaceRef): boolean {
  return place.state_category === "TERMINAL" || place.state_category === "FAILED";
}

export function emptyStatePlaceMessage(
  usesRetainedWorkItems: boolean,
  tokenCount: number,
): string {
  if (!usesRetainedWorkItems) {
    return NO_CURRENT_WORK_IN_PLACE_COPY;
  }

  if (tokenCount > 0) {
    return SELECTED_TICK_WORK_UNAVAILABLE_COPY;
  }

  return NO_WORK_RECORDED_AT_SELECTED_TICK_COPY;
}

export function normalizeDetailText(value: string | undefined): string | undefined {
  const trimmed = value?.trim();
  return trimmed ? trimmed : undefined;
}

export function RequestAuthoredText({ value }: { value: string }) {
  const blocks = parseRequestAuthoredBlocks(value);

  return (
    <div className={REQUEST_AUTHORED_TEXT_CLASS}>
      {blocks.map((block, index) => renderRequestAuthoredBlock(block, index))}
    </div>
  );
}

function parseRequestAuthoredBlocks(value: string): RequestAuthoredBlock[] {
  const lines = value.split(/\r?\n/);
  const blocks: RequestAuthoredBlock[] = [];

  for (let lineIndex = 0; lineIndex < lines.length;) {
    const line = lines[lineIndex];

    if (!line.trim()) {
      lineIndex += 1;
      continue;
    }

    const fencedCodeMatch = line.match(/^```([^\s`]+)?\s*$/);
    if (fencedCodeMatch) {
      const codeLines: string[] = [];
      lineIndex += 1;

      while (lineIndex < lines.length && !/^```\s*$/.test(lines[lineIndex])) {
        codeLines.push(lines[lineIndex]);
        lineIndex += 1;
      }

      if (lineIndex < lines.length) {
        lineIndex += 1;
      }

      blocks.push({
        code: codeLines.join("\n"),
        language: fencedCodeMatch[1],
        type: "code-block",
      });
      continue;
    }

    const headingMatch = line.match(/^(#{1,6})\s+(.*)$/);
    if (headingMatch) {
      blocks.push({
        level: headingMatch[1].length as RequestAuthoredHeadingBlock["level"],
        text: headingMatch[2],
        type: "heading",
      });
      lineIndex += 1;
      continue;
    }

    const unorderedListMatch = line.match(/^[-*+]\s+(.*)$/);
    if (unorderedListMatch) {
      const items: string[] = [];

      while (lineIndex < lines.length) {
        const listItemMatch = lines[lineIndex].match(/^[-*+]\s+(.*)$/);
        if (!listItemMatch) {
          break;
        }

        items.push(listItemMatch[1]);
        lineIndex += 1;
      }

      blocks.push({ items, type: "unordered-list" });
      continue;
    }

    const orderedListMatch = line.match(/^\d+\.\s+(.*)$/);
    if (orderedListMatch) {
      const items: string[] = [];

      while (lineIndex < lines.length) {
        const listItemMatch = lines[lineIndex].match(/^\d+\.\s+(.*)$/);
        if (!listItemMatch) {
          break;
        }

        items.push(listItemMatch[1]);
        lineIndex += 1;
      }

      blocks.push({ items, type: "ordered-list" });
      continue;
    }

    const paragraphLines: string[] = [];

    while (lineIndex < lines.length && shouldContinueParagraph(lines[lineIndex])) {
      paragraphLines.push(lines[lineIndex]);
      lineIndex += 1;
    }

    blocks.push({
      text: paragraphLines.join("\n"),
      type: "paragraph",
    });
  }

  return blocks;
}

function shouldContinueParagraph(line: string): boolean {
  if (!line.trim()) {
    return false;
  }

  return !/^(#{1,6})\s+/.test(line)
    && !/^[-*+]\s+/.test(line)
    && !/^\d+\.\s+/.test(line)
    && !/^```([^\s`]+)?\s*$/.test(line);
}

function renderRequestAuthoredBlock(block: RequestAuthoredBlock, index: number) {
  switch (block.type) {
    case "code-block":
      return (
        <pre key={`code-block-${index}`}>
          <code data-language={block.language}>{block.code}</code>
        </pre>
      );
    case "heading": {
      const HeadingTag = `h${block.level}` as const;
      return (
        <HeadingTag className="m-0" key={`heading-${index}`}>
          {renderInlineMarkdown(block.text)}
        </HeadingTag>
      );
    }
    case "ordered-list":
      return (
        <ol key={`ordered-list-${index}`}>
          {block.items.map((item, itemIndex) => (
            <li className="whitespace-pre-wrap" key={`ordered-list-item-${index}-${itemIndex}`}>
              {renderInlineMarkdown(item)}
            </li>
          ))}
        </ol>
      );
    case "unordered-list":
      return (
        <ul key={`unordered-list-${index}`}>
          {block.items.map((item, itemIndex) => (
            <li className="whitespace-pre-wrap" key={`unordered-list-item-${index}-${itemIndex}`}>
              {renderInlineMarkdown(item)}
            </li>
          ))}
        </ul>
      );
    case "paragraph":
      return (
        <p className="m-0 whitespace-pre-wrap" key={`paragraph-${index}`}>
          {renderInlineMarkdown(block.text)}
        </p>
      );
  }
}

function renderInlineMarkdown(value: string): ReactNode[] {
  const segments: ReactNode[] = [];
  const inlineCodePattern = /`([^`]+)`/g;
  let lastIndex = 0;
  let match = inlineCodePattern.exec(value);

  while (match) {
    if (match.index > lastIndex) {
      segments.push(value.slice(lastIndex, match.index));
    }

    segments.push(
      <code key={`inline-code-${match.index}`}>{match[1]}</code>,
    );
    lastIndex = inlineCodePattern.lastIndex;
    match = inlineCodePattern.exec(value);
  }

  if (lastIndex < value.length) {
    segments.push(value.slice(lastIndex));
  }

  return segments.map((segment, index) => {
    if (typeof segment === "string") {
      return <Fragment key={`inline-text-${index}`}>{segment}</Fragment>;
    }

    return segment;
  });
}
