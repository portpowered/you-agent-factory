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

      return {
        data: undefined,
        error: null,
        status: "pending",
      };
    }, [eventFactoryDocument]);

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
  const version = eventFactory.version;
  if (
    version == null ||
    typeof version !== "object" ||
    (typeof version.logical !== "string" &&
      typeof version.logical !== "number") ||
    typeof version.physical !== "string"
  ) {
    return null;
  }

  return {
    ...eventFactory,
    version: {
      logical: String(version.logical),
      physical: version.physical,
    },
  };
}
