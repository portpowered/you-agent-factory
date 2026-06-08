// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: canonical name transport cases share one mocked fetch seam.
import { saveFactoryForSessionDocument } from "./api";

describe("saveFactoryForSessionDocument", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("applies canonicalFactoryName on REPLACE_CURRENT saves when the editable payload name drifted", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          name: "alpha",
          workers: [],
          workstations: [],
          workTypes: [],
          version: {
            logical: "2",
            physical: "2026-05-18T14:42:00Z",
          },
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
          statusText: "OK",
        },
      ),
    );

    await saveFactoryForSessionDocument(
      {
        canonicalFactoryName: "alpha",
        factoryDefinition: {
          name: "imported-factory",
          workers: [],
          workstations: [],
          workTypes: [],
        },
        mode: "REPLACE_CURRENT",
      },
      {
        fetch,
        sessionID: "session-2",
      },
    );

    expect(fetch).toHaveBeenCalledWith(
      "/factory-sessions/session-2/factory",
      expect.objectContaining({
        body: JSON.stringify({
          mode: "REPLACE_CURRENT",
          factory: {
            name: "alpha",
            workers: [],
            workstations: [],
            workTypes: [],
          },
        }),
        method: "PUT",
      }),
    );
  });

  it("applies canonicalFactoryName on UPSERT_NAMED_AND_ACTIVATE saves when the imported payload name drifted", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          name: "Imported Factory Name-2",
          workers: [],
          workstations: [],
          workTypes: [],
          version: {
            logical: "2",
            physical: "2026-05-18T14:42:00Z",
          },
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
          statusText: "OK",
        },
      ),
    );

    await saveFactoryForSessionDocument(
      {
        canonicalFactoryName: "Imported Factory Name-2",
        factoryDefinition: {
          name: "Imported Factory Name",
          workers: [],
          workstations: [],
          workTypes: [],
        },
        includeVersion: false,
        mode: "UPSERT_NAMED_AND_ACTIVATE",
      },
      {
        fetch,
        sessionID: "session-2",
      },
    );

    expect(fetch).toHaveBeenCalledWith(
      "/factory-sessions/session-2/factory",
      expect.objectContaining({
        body: JSON.stringify({
          mode: "UPSERT_NAMED_AND_ACTIVATE",
          factory: {
            name: "Imported Factory Name-2",
            workers: [],
            workstations: [],
            workTypes: [],
          },
        }),
        method: "PUT",
      }),
    );
  });

  it("delegates saveFactoryForSessionDocument to session-factory with explicit save mode", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          name: "Imported Factory",
          workers: [],
          workstations: [],
          workTypes: [],
          version: {
            logical: "2",
            physical: "2026-05-18T14:42:00Z",
          },
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
          statusText: "OK",
        },
      ),
    );

    await saveFactoryForSessionDocument(
      {
        factoryDefinition: {
          name: "Imported Factory",
          workers: [],
          workstations: [],
          workTypes: [],
        },
        includeVersion: false,
        mode: "UPSERT_NAMED_AND_ACTIVATE",
      },
      {
        fetch,
        sessionID: "session-2",
      },
    );

    expect(fetch).toHaveBeenCalledWith(
      "/factory-sessions/session-2/factory",
      expect.objectContaining({
        body: JSON.stringify({
          mode: "UPSERT_NAMED_AND_ACTIVATE",
          factory: {
            name: "Imported Factory",
            workers: [],
            workstations: [],
            workTypes: [],
          },
        }),
        method: "PUT",
      }),
    );
  });
});
