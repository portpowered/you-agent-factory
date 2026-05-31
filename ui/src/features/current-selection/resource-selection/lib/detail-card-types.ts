import type { FactoryResource } from "../../../../api/events/types";

export interface ResourceDetailCardProps {
  locale?: string | null;
  resourceName: string;
  tokenCount?: number | null;
  widgetId?: string;
}

export type ResourceDetailState =
  | { status: "loading" }
  | { status: "error"; errorMessage: string }
  | { status: "empty" }
  | {
      status: "ready";
      resource: FactoryResource;
      workerNames: string[];
      workstationNames: string[];
    };
