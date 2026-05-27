import type { OnError } from "@xyflow/react";

export const failOnTraceReactFlowError: OnError = (id, message) => {
  throw new Error(`Trace React Flow error ${id}: ${message}`);
};
