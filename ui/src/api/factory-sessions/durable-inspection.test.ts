import {
  listDurableFactorySessionArtifacts,
  listDurableFactorySessionDispatches,
} from "./durable-inspection";

describe("durable factory session inspection API", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("loads durable dispatch summaries from the typed list surface", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          dispatches: [
            {
              dispatchKind: "PETRI_TRANSITION",
              id: "disp-petri-success-001",
              label: "plan-task",
              status: "COMPLETED",
            },
          ],
          sessionId: "dur-sess-petri-success-001",
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      listDurableFactorySessionDispatches("dur-sess-petri-success-001"),
    ).resolves.toEqual([
      {
        dispatchKind: "PETRI_TRANSITION",
        id: "disp-petri-success-001",
        label: "plan-task",
        status: "COMPLETED",
      },
    ]);
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions/dur-sess-petri-success-001/dispatches",
      expect.objectContaining({ method: "GET" }),
    );
  });

  it("loads durable artifact summaries from the typed list surface", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          artifacts: [
            {
              id: "art-petri-final-001",
              kind: "FINAL_RESULT",
              label: "Triage summary",
              visibility: "PUBLIC",
            },
          ],
          sessionId: "dur-sess-petri-success-001",
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      listDurableFactorySessionArtifacts("dur-sess-petri-success-001"),
    ).resolves.toEqual([
      {
        id: "art-petri-final-001",
        kind: "FINAL_RESULT",
        label: "Triage summary",
        visibility: "PUBLIC",
      },
    ]);
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions/dur-sess-petri-success-001/artifacts",
      expect.objectContaining({ method: "GET" }),
    );
  });
});
