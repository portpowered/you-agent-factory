import { describe, expect, it } from "vitest";

import { currentFactorySessionPath } from "../api/session-routing";
import {
  buildSessionFactoryActivationPutBody,
  defaultSessionFactoryVersion,
  incrementedSessionFactoryVersion,
  incrementSessionFactoryVersion,
  isSessionFactoryRequest,
  mockGetSessionFactory,
  mockPutSessionFactory,
  sessionFactoryImportActivationDocument,
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
      name: "Session Current Name",
      workTypes: [
        { name: "story", states: [{ name: "new", type: "INITIAL" }] },
      ],
      workers: [],
      workstations: [],
      version: incrementedSessionFactoryVersion,
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
  });
});
