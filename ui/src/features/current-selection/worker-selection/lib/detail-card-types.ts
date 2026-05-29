import type { FactoryWorker } from "../../../../api/events/types";

export interface WorkerDetailCardProps {
  locale?: string | null;
  widgetId?: string;
  workerName: string;
}

export type WorkerDetailState =
  | { status: "loading" }
  | { status: "error"; errorMessage: string }
  | { status: "empty" }
  | {
      status: "ready";
      worker: FactoryWorker;
      workstationNames: string[];
    };
