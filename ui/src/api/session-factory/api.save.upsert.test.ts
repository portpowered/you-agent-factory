import { saveSessionFactory } from "./api";
import { SessionFactoryAPIError } from "./errors";

describe("saveSessionFactory upsert and transport", () => {
  it("issues PUT with UPSERT_NAMED_AND_ACTIVATE on a non-default session without version", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          name: "Imported Factory",
          workers: [],
          workstations: [],
          workTypes: [],
          version: {
            logical: "1",
            physical: "2026-05-18T14:41:00Z",
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

    await saveSessionFactory(
      {
        sessionID: "session-2",
        mode: "UPSERT_NAMED_AND_ACTIVATE",
        factory: {
          name: "Imported Factory",
          workers: [],
          workstations: [],
          workTypes: [],
        },
        includeVersion: false,
      },
      { fetch },
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

  it("throws SessionFactoryAPIError for network failures", async () => {
    await expect(
      saveSessionFactory(
        {
          sessionID: "~default",
          factory: {
            name: "Current Factory",
            workers: [],
            workstations: [],
            workTypes: [],
          },
        },
        {
          fetch: vi.fn().mockRejectedValue(new Error("socket closed")),
        },
      ),
    ).rejects.toBeInstanceOf(SessionFactoryAPIError);
  });
});
