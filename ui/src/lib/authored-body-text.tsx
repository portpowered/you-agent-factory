import { Fragment } from "react";
import type { ReactNode } from "react";
import { DASHBOARD_BODY_TEXT_CLASS } from "../components/ui/dashboard-typography";
import { cn } from "./cn";

export const REQUEST_AUTHORED_TEXT_CLASS = cn(
  "grid gap-3 rounded-lg border border-af-border bg-af-surface-raised p-3 [overflow-wrap:anywhere] [&_code]:rounded-sm [&_code]:bg-af-overlay [&_code]:px-1 [&_code]:py-0.5 [&_h1]:text-xl [&_h1]:font-semibold [&_h2]:text-lg [&_h2]:font-semibold [&_h3]:text-base [&_h3]:font-semibold [&_ol]:m-0 [&_ol]:list-decimal [&_ol]:pl-5 [&_pre]:m-0 [&_pre]:overflow-x-auto [&_pre]:rounded-lg [&_pre]:border [&_pre]:border-af-border [&_pre]:bg-af-surface-subtle [&_pre]:p-3 [&_pre_code]:bg-transparent [&_pre_code]:p-0 [&_ul]:m-0 [&_ul]:list-disc [&_ul]:pl-5",
  DASHBOARD_BODY_TEXT_CLASS,
);

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

export function AuthoredBodyText({
  className,
  value,
}: {
  className?: string;
  value: string;
}) {
  const blocks = parseRequestAuthoredBlocks(value);

  return (
    <div className={cn(REQUEST_AUTHORED_TEXT_CLASS, className)}>
      {blocks.map((block, index) => renderRequestAuthoredBlock(block, index))}
    </div>
  );
}

export function RequestAuthoredText({ value }: { value: string }) {
  return <AuthoredBodyText value={value} />;
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
          {stableListKeys(block.items).map(({ item, key }) => (
            <li className="whitespace-pre-wrap" key={`ordered-list-item-${index}-${key}`}>
              {renderInlineMarkdown(item)}
            </li>
          ))}
        </ol>
      );
    case "unordered-list":
      return (
        <ul key={`unordered-list-${index}`}>
          {stableListKeys(block.items).map(({ item, key }) => (
            <li className="whitespace-pre-wrap" key={`unordered-list-item-${index}-${key}`}>
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

  const seenStringSegments = new Map<string, number>();
  return segments.map((segment) => {
    if (typeof segment === "string") {
      const occurrence = (seenStringSegments.get(segment) ?? 0) + 1;
      seenStringSegments.set(segment, occurrence);
      return <Fragment key={`inline-text-${segment}-${occurrence}`}>{segment}</Fragment>;
    }

    return segment;
  });
}

function stableListKeys(items: string[]): Array<{ item: string; key: string }> {
  const occurrences = new Map<string, number>();
  return items.map((item) => {
    const occurrence = (occurrences.get(item) ?? 0) + 1;
    occurrences.set(item, occurrence);
    return {
      item,
      key: `${item}-${occurrence}`,
    };
  });
}
