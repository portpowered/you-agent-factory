import { useMemo } from "react";
import type {
  CurrentFactoryDefinitionError,
  CurrentFactoryDocument,
} from "../../../api/current-factory-definition";
import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { preserveExistingBundledFilesWhenAbsent } from "../../../api/factory-definition";
import { useCurrentFactoryDocument } from "../../current-factory-definition/hooks/useCurrentFactoryDefinition";

export interface CurrentActivityFactoryDocumentQuery {
  data?: CurrentFactoryDocument;
  error?: CurrentFactoryDefinitionError | null;
  status: "error" | "pending" | "success";
}

export interface CurrentActivityFactoryDocumentState {
  /**
   * Transitional current-factory document read. Graph render state should prefer
   * event-computed factory data; this document remains the edit/save baseline for
   * version metadata and bundled files until the stream contract carries them.
   */
  currentFactoryDocument?: CurrentFactoryDocument;
  editableDefinitionQuery: CurrentActivityFactoryDocumentQuery;
}

export function useCurrentActivityFactoryDocumentState({
  enabled = true,
  eventFactory,
}: {
  enabled?: boolean;
  eventFactory?: DashboardSnapshot["factory"] | null;
} = {}): CurrentActivityFactoryDocumentState {
  const editableDefinitionQuery = useCurrentFactoryDocument(enabled);
  const eventFactoryDocument = useMemo(
    () =>
      eventFactory
        ? toCurrentFactoryDocumentFromEventFactory(
            eventFactory,
            editableDefinitionQuery.data,
          )
        : null,
    [editableDefinitionQuery.data, eventFactory],
  );
  const currentFactoryDocument =
    eventFactoryDocument ?? editableDefinitionQuery.data;
  const resolvedDefinitionQuery =
    useMemo((): CurrentActivityFactoryDocumentQuery => {
      if (!eventFactoryDocument) {
        return editableDefinitionQuery;
      }

      return {
        data: eventFactoryDocument,
        error: null,
        status: "success",
      };
    }, [editableDefinitionQuery, eventFactoryDocument]);

  return useMemo(
    () => ({
      currentFactoryDocument,
      editableDefinitionQuery: resolvedDefinitionQuery,
    }),
    [currentFactoryDocument, resolvedDefinitionQuery],
  );
}

function toCurrentFactoryDocumentFromEventFactory(
  eventFactory: NonNullable<DashboardSnapshot["factory"]>,
  cachedDocument: CurrentFactoryDocument | undefined,
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

  const withBundledFiles = preserveExistingBundledFilesWhenAbsent(
    eventFactory,
    cachedDocument,
  );

  return {
    ...withBundledFiles,
    version: {
      logical: String(version.logical),
      physical: version.physical,
    },
  };
}
