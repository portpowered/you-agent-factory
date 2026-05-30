import { describe, expect, it } from "vitest";

import { currentFactorySessionPath } from "../api/session-routing";
import { baseFactoryDefinition } from "./graph-editor-harness";
import {
  buildSessionFactoryActivationPutBody,
  defaultSessionFactoryVersion,
  incrementedSessionFactoryVersion,
  incrementSessionFactoryVersion,
  isSessionFactoryRequest,
  mergeImportedFactoryIntoSessionDocument,
  mockGetSessionFactory,
  mockPutSessionFactory,
  sessionFactoryImportActivationDocument,
  sessionFactoryNamedExportDocument,
} from "./session-factory-mocks";

describe("session-factory-mocks", () => {
  it("mockGetSessionFactory returns OpenAPI factory document shape with version", async () => {
    const response = mockGetSessionFactory();
    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toEqual(
      sessionFactoryImportActivationDocument,
    );
  });

  it("mockPutSessionFactory returns success, stale version, and not-idle error bodies", async () => {
    const success = mockPutSessionFactory();
    expect(success.status).toBe(200);
    await expect(success.json()).resolves.toEqual({
      ...sessionFactoryImportActivationDocument,
      version: incrementedSessionFactoryVersion,
    });

    const stale = mockPutSessionFactory({ mode: "stale_version" });
    expect(stale.status).toBe(409);
    await expect(stale.json()).resolves.toEqual({
      code: "STALE_FACTORY_VERSION",
      message: "The editable definition is stale.",
    });

    const notIdle = mockPutSessionFactory({ mode: "factory_not_idle" });
    expect(notIdle.status).toBe(409);
    await expect(notIdle.json()).resolves.toEqual({
      code: "FACTORY_NOT_IDLE",
      message: "Current factory runtime must be idle before activation.",
    });
  });

  it("builds activation PUT bodies with session name preservation and incremented version", () => {
    expect(
      buildSessionFactoryActivationPutBody({
        sessionName: "Session Current Name",
        importedFactory: {
          name: "Imported Factory Name",
          workTypes: [
            { name: "story", states: [{ name: "new", type: "INITIAL" }] },
          ],
          workers: [],
          workstations: [],
        },
      }),
    ).toEqual({
      mode: "REPLACE_CURRENT",
      factory: {
        name: "Session Current Name",
        workTypes: [
          { name: "story", states: [{ name: "new", type: "INITIAL" }] },
        ],
        workers: [],
        workstations: [],
        version: incrementedSessionFactoryVersion,
      },
    });
  });

  it("increments session factory versions like the current-factory client", () => {
    expect(
      incrementSessionFactoryVersion(defaultSessionFactoryVersion),
    ).toEqual(incrementedSessionFactoryVersion);
  });

  it("matches session-scoped GET and PUT paths", () => {
    expect(
      isSessionFactoryRequest(
        currentFactorySessionPath("session-2"),
        "GET",
        "session-2",
      ),
    ).toBe(true);
    expect(isSessionFactoryRequest(currentFactorySessionPath(), "PUT")).toBe(
      true,
    );
    expect(
      isSessionFactoryRequest(
        "/factory-sessions/session-2/work",
        "GET",
        "session-2",
      ),
    ).toBe(false);
    expect(
      isSessionFactoryRequest(currentFactorySessionPath("session-2"), "POST"),
    ).toBe(false);
  });

  it("mockGetSessionFactory honors custom document and status options", async () => {
    const response = mockGetSessionFactory({
      document: sessionFactoryNamedExportDocument,
      status: 503,
      statusText: "Service Unavailable",
    });

    expect(response.status).toBe(503);
    expect(response.statusText).toBe("Service Unavailable");
    await expect(response.json()).resolves.toEqual(
      sessionFactoryNamedExportDocument,
    );
  });

  it("mockPutSessionFactory honors custom error copy and success payloads", async () => {
    const stale = mockPutSessionFactory({
      mode: "stale_version",
      staleVersionMessage: "Version mismatch.",
    });
    await expect(stale.json()).resolves.toEqual({
      code: "STALE_FACTORY_VERSION",
      message: "Version mismatch.",
    });

    const notIdle = mockPutSessionFactory({
      factoryNotIdleMessage: "Factory is running.",
      mode: "factory_not_idle",
    });
    await expect(notIdle.json()).resolves.toEqual({
      code: "FACTORY_NOT_IDLE",
      message: "Factory is running.",
    });

    const success = mockPutSessionFactory({
      responseDocument: sessionFactoryNamedExportDocument,
      statusText: "OK",
    });
    expect(success.statusText).toBe("OK");
    await expect(success.json()).resolves.toEqual(
      sessionFactoryNamedExportDocument,
    );
  });

  it("mergeImportedFactoryIntoSessionDocument preserves session name and applies version", () => {
    expect(
      mergeImportedFactoryIntoSessionDocument(
        sessionFactoryImportActivationDocument,
        baseFactoryDefinition,
        incrementedSessionFactoryVersion,
      ),
    ).toEqual({
      ...baseFactoryDefinition,
      name: sessionFactoryImportActivationDocument.name,
      version: incrementedSessionFactoryVersion,
    });
  });

  it("incrementSessionFactoryVersion leaves non-ISO physical timestamps unchanged", () => {
    expect(
      incrementSessionFactoryVersion({
        logical: "1",
        physical: "not-a-timestamp",
      }),
    ).toEqual({
      logical: "2",
      physical: "not-a-timestamp",
    });
  });
});
