import type { ReactNode } from "react";

import { cx } from "../../components/dashboard/classnames";

export const GRAPH_SEMANTIC_ICON_KINDS = [
  "queue",
  "processing",
  "terminal",
  "failed",
  "resource",
  "constraint",
  "limit",
  "workstation",
  "repeater",
  "cron",
  "exhaustion",
  "active-work",
] as const;

export type GraphSemanticIconKind = (typeof GRAPH_SEMANTIC_ICON_KINDS)[number];

interface GraphSemanticIconDefinition {
  label: string;
  paths: ReactNode;
}

export interface GraphSemanticIconProps {
  className?: string;
  kind: GraphSemanticIconKind | (string & {});
  label?: string;
}

const DEFAULT_ICON_CLASS_NAME = "h-4 w-4 shrink-0 text-af-ink/72";
const UNKNOWN_ICON_LABEL = "Unknown graph semantic";

const GRAPH_SEMANTIC_ICON_DEFINITIONS = {
  "active-work": {
    label: "Active work",
    paths: (
      <>
        <path d="M8 5v14l11-7-11-7Z" />
        <path d="M4.5 8.5v7" />
      </>
    ),
  },
  constraint: {
    label: "Constraint",
    paths: (
      <>
        <rect height="9" rx="2" width="14" x="5" y="10" />
        <path d="M8 10V8a4 4 0 0 1 8 0v2" />
        <path d="M12 13.5v2" />
      </>
    ),
  },
  cron: {
    label: "Cron workstation",
    paths: (
      <>
        <circle cx="12" cy="12" r="8" />
        <path d="M12 7.5V12l3 2" />
      </>
    ),
  },
  exhaustion: {
    label: "Exhaustion rule",
    paths: (
      <>
        <path d="M12 4 21 19H3L12 4Z" />
        <path d="M12 9v4" />
        <path d="M12 16.5h.01" />
      </>
    ),
  },
  failed: {
    label: "Failed state",
    paths: (
      <>
        <path d="M8.5 3.5h7L20.5 8.5v7l-5 5h-7l-5-5v-7l5-5Z" />
        <path d="m9 9 6 6" />
        <path d="m15 9-6 6" />
      </>
    ),
  },
  limit: {
    label: "Limit",
    paths: (
      <>
        <path d="M5 17a7 7 0 0 1 14 0" />
        <path d="M12 17l4-6" />
        <path d="M8 19h8" />
      </>
    ),
  },
  processing: {
    label: "Processing state",
    paths: (
      <>
        <path d="M18.5 9A7 7 0 0 0 6.2 6.2L4.5 8" />
        <path d="M4.5 4.5V8h3.5" />
        <path d="M5.5 15a7 7 0 0 0 12.3 2.8L19.5 16" />
        <path d="M19.5 19.5V16H16" />
      </>
    ),
  },
  queue: {
    label: "Queue state",
    paths: (
      <>
        <path d="M5 7h11" />
        <path d="M5 12h14" />
        <path d="M5 17h8" />
        <path d="m16 15 3 2-3 2" />
      </>
    ),
  },
  repeater: {
    label: "Repeater workstation",
    paths: (
      <>
        <path d="M17 7h-6a5 5 0 0 0 0 10h1" />
        <path d="m15 4 3 3-3 3" />
        <path d="M7 17h6a5 5 0 0 0 0-10h-1" />
        <path d="m9 20-3-3 3-3" />
      </>
    ),
  },
  resource: {
    label: "Resource",
    paths: (
      <>
        <ellipse cx="12" cy="6" rx="6.5" ry="3" />
        <path d="M5.5 6v6c0 1.7 2.9 3 6.5 3s6.5-1.3 6.5-3V6" />
        <path d="M5.5 12v6c0 1.7 2.9 3 6.5 3s6.5-1.3 6.5-3v-6" />
      </>
    ),
  },
  terminal: {
    label: "Terminal state",
    paths: (
      <>
        <circle cx="12" cy="12" r="8" />
        <path d="m8.5 12 2.3 2.3 4.7-5" />
      </>
    ),
  },
  workstation: {
    label: "Workstation",
    paths: (
      <>
        <rect height="10" rx="2" width="14" x="5" y="5" />
        <path d="M9 19h6" />
        <path d="M12 15v4" />
      </>
    ),
  },
} satisfies Record<GraphSemanticIconKind, GraphSemanticIconDefinition>;

function unknownIconPaths(): ReactNode {
  return (
    <>
      <path d="M12 3.5 20.5 8.2v7.6L12 20.5 3.5 15.8V8.2L12 3.5Z" />
      <path d="M9.5 9a2.6 2.6 0 1 1 3.2 2.5c-.5.2-.7.6-.7 1.2v.3" />
      <path d="M12 16.5h.01" />
    </>
  );
}

export function graphSemanticIconLabel(kind: GraphSemanticIconProps["kind"]): string {
  return (
    GRAPH_SEMANTIC_ICON_DEFINITIONS[kind as GraphSemanticIconKind]?.label ??
    UNKNOWN_ICON_LABEL
  );
}

export function GraphSemanticIcon({
  className,
  kind,
  label,
}: GraphSemanticIconProps) {
  const definition = GRAPH_SEMANTIC_ICON_DEFINITIONS[kind as GraphSemanticIconKind];
  const accessibleLabel = label ?? definition?.label ?? UNKNOWN_ICON_LABEL;

  return (
    <svg
      aria-label={accessibleLabel}
      className={cx(DEFAULT_ICON_CLASS_NAME, className)}
      data-graph-semantic-icon={definition ? kind : "unknown"}
      fill="none"
      focusable="false"
      role="img"
      stroke="currentColor"
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth="1.8"
      viewBox="0 0 24 24"
      xmlns="http://www.w3.org/2000/svg"
    >
      {definition?.paths ?? unknownIconPaths()}
    </svg>
  );
}
