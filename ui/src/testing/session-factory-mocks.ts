import type {
  CanonicalFactoryDefinition,
  CurrentFactoryDocument,
  CurrentFactoryVersion,
} from "../api/current-factory-definition";
import type { components } from "../api/generated/openapi";
import type { FactoryValue } from "../api/named-factory";
import { currentFactorySessionPath } from "../api/session-routing";
import { baseFactoryDefinition } from "./graph-editor-harness";

export type SessionFactoryDocument = CurrentFactoryDocument;
export type SessionFactoryVersion = CurrentFactoryVersion;

export type SessionFactoryPutMode =
  | "success"
  | "stale_version"
  | "factory_not_idle";

export const defaultSessionFactoryVersion: SessionFactoryVersion = {
  logical: "9",
  physical: "2026-05-18T14:25:00Z",
};

export const incrementedSessionFactoryVersion: SessionFactoryVersion = {
  logical: "10",
  physical: "2026-05-18T14:25:00.001Z",
};

export const sessionFactoryImportActivationDocument: SessionFactoryDocument = {
  name: "Session Current Name",
  workTypes: [],
  workers: [],
  workstations: [],
  version: defaultSessionFactoryVersion,
};

export const sessionFactoryNamedExportDocument: SessionFactoryDocument = {
  ...baseFactoryDefinition,
  name: "semantic-workflow",
  version: defaultSessionFactoryVersion,
};

export interface MockGetSessionFactoryOptions {
  document?: SessionFactoryDocument;
  status?: number;
  statusText?: string;
}

export interface MockPutSessionFactoryOptions {
  mode?: SessionFactoryPutMode;
  responseDocument?: SessionFactoryDocument;
  staleVersionMessage?: string;
  factoryNotIdleMessage?: string;
  status?: number;
  statusText?: string;
}

export interface SessionFactoryActivationPutBodyOptions {
  sessionName: string;
  importedFactory: FactoryValue;
  version?: SessionFactoryVersion;
}

export function mockGetSessionFactory(
  options: MockGetSessionFactoryOptions = {},
): Response {
  const document = options.document ?? sessionFactoryImportActivationDocument;
  return jsonResponse(document, options.status ?? 200, options.statusText);
}

export function mockPutSessionFactory(
  options: MockPutSessionFactoryOptions = {},
): Response {
  const mode = options.mode ?? "success";

  if (mode === "stale_version") {
    return jsonResponse(
      {
        code: "STALE_FACTORY_VERSION",
        message:
          options.staleVersionMessage ?? "The editable definition is stale.",
      },
      options.status ?? 409,
      options.statusText ?? "Conflict",
    );
  }

  if (mode === "factory_not_idle") {
    return jsonResponse(
      {
        code: "FACTORY_NOT_IDLE",
        message:
          options.factoryNotIdleMessage ??
          "Current factory runtime must be idle before activation.",
      },
      options.status ?? 409,
      options.statusText ?? "Conflict",
    );
  }

  const responseDocument = options.responseDocument ?? {
    ...sessionFactoryImportActivationDocument,
    version: incrementedSessionFactoryVersion,
  };

  return jsonResponse(
    responseDocument,
    options.status ?? 200,
    options.statusText,
  );
}

export function buildSessionFactoryActivationPutBody(
  options: SessionFactoryActivationPutBodyOptions,
): components["schemas"]["SaveFactoryForSessionRequest"] {
  return {
    mode: "REPLACE_CURRENT",
    factory: {
      name: options.sessionName,
      workTypes: options.importedFactory.workTypes,
      workers: options.importedFactory.workers,
      workstations: options.importedFactory.workstations,
      version: options.version ?? incrementedSessionFactoryVersion,
    },
  };
}

export function parseSessionFactoryPutFactory(
  body: string,
): components["schemas"]["Factory"] {
  const parsed = JSON.parse(body) as
    | components["schemas"]["SaveFactoryForSessionRequest"]
    | components["schemas"]["Factory"];
  if ("factory" in parsed && parsed.factory) {
    return parsed.factory;
  }
  return parsed as components["schemas"]["Factory"];
}

export function incrementSessionFactoryVersion(
  version: SessionFactoryVersion,
): SessionFactoryVersion {
  return {
    logical: (BigInt(version.logical) + 1n).toString(),
    physical: incrementSessionFactoryVersionPhysical(version.physical),
  };
}

export function isSessionFactoryRequest(
  path: string,
  method: string,
  sessionID?: string | null,
): boolean {
  return (
    path === currentFactorySessionPath(sessionID) &&
    (method === "GET" || method === "PUT")
  );
}

export function mergeImportedFactoryIntoSessionDocument(
  currentDocument: SessionFactoryDocument,
  importedFactory: CanonicalFactoryDefinition,
  version: SessionFactoryVersion = incrementedSessionFactoryVersion,
): SessionFactoryDocument {
  return {
    ...currentDocument,
    ...importedFactory,
    name: currentDocument.name,
    version,
  };
}

function incrementSessionFactoryVersionPhysical(physical: string): string {
  const parsed = Date.parse(physical);
  if (!Number.isFinite(parsed)) {
    return physical;
  }
  return new Date(parsed + 1).toISOString();
}

function jsonResponse(
  body: unknown,
  status = 200,
  statusText?: string,
): Response {
  return new Response(JSON.stringify(body), {
    headers: {
      "Content-Type": "application/json",
    },
    status,
    statusText,
  });
}
