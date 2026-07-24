import type { GraphNodeHandleTone } from "@you-agent-factory/components/graphs";

export function graphHandleToneFromId(handleId: string): GraphNodeHandleTone {
  if (handleId.includes("resource")) {
    return "resource";
  }

  if (
    handleId.includes("worker-assignment") ||
    handleId.includes("worker-input")
  ) {
    return "worker";
  }

  if (handleId.includes("on-continue")) {
    return "continue";
  }

  if (handleId.includes("on-failure")) {
    return "failure";
  }

  if (handleId.includes("on-rejection")) {
    return "rejection";
  }

  if (handleId.includes("output")) {
    return "output";
  }

  if (handleId.includes("input")) {
    return "input";
  }

  if (handleId.includes("assignment")) {
    return "assignment";
  }

  return "default";
}
