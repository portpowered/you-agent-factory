import { useMemo } from "react";
import type {
  CurrentFactoryDefinitionError,
  CurrentFactoryDocument,
} from "../../../api/current-factory-definition";
import type { DashboardSnapshot } from "../../../api/dashboard/types";

export interface CurrentActivityFactoryDocumentQuery {
  data?: CurrentFactoryDocument;
  error?: CurrentFactoryDefinitionError | null;
  status: "error" | "pending" | "success";
}

export interface CurrentActivityFactoryDocumentState {
  /** Event-computed factory document used as the edit/save baseline. */
  currentFactoryDocument?: CurrentFactoryDocument;
  editableDefinitionQuery: CurrentActivityFactoryDocumentQuery;
}

type VersionedEventFactory = NonNullable<DashboardSnapshot["factory"]> & {
  version: {
    logical: string | number;
    physical: string;
  };
};

export function useCurrentActivityFactoryDocumentState({
  eventFactory,
}: {
  eventFactory?: DashboardSnapshot["factory"] | null;
} = {}): CurrentActivityFactoryDocumentState {
  const eventFactoryDocument = useMemo(
    () =>
      eventFactory
        ? toCurrentFactoryDocumentFromEventFactory(eventFactory)
        : null,
    [eventFactory],
  );

  const editableDefinitionQuery =
    useMemo((): CurrentActivityFactoryDocumentQuery => {
      if (eventFactoryDocument) {
        return {
          data: eventFactoryDocument,
          error: null,
          status: "success",
        };
      }

      if (hasVersionedEventFactory(eventFactory)) {
        return {
          data: undefined,
          error: null,
          status: "success",
        };
      }

      return {
        data: undefined,
        error: null,
        status: "pending",
      };
    }, [eventFactory, eventFactoryDocument]);

  return useMemo(
    () => ({
      currentFactoryDocument: eventFactoryDocument ?? undefined,
      editableDefinitionQuery,
    }),
    [editableDefinitionQuery, eventFactoryDocument],
  );
}

function toCurrentFactoryDocumentFromEventFactory(
  eventFactory: NonNullable<DashboardSnapshot["factory"]>,
): CurrentFactoryDocument | null {
  if (
    !hasVersionedEventFactory(eventFactory) ||
    eventFactory.activation == null
  ) {
    return null;
  }

  const version = eventFactory.version;

  return {
    ...eventFactory,
    activation: eventFactory.activation,
    version: {
      logical: String(version.logical),
      physical: version.physical,
    },
  };
}

function hasVersionedEventFactory(
  eventFactory?: DashboardSnapshot["factory"] | null,
): eventFactory is VersionedEventFactory {
  const version = eventFactory?.version;
  return (
    version != null &&
    typeof version === "object" &&
    (typeof version.logical === "string" ||
      typeof version.logical === "number") &&
    typeof version.physical === "string"
  );
}
