import {
  isStageSubmitWorkFileAPIError,
  StageSubmitWorkFileAPIError,
  stageSubmitWorkFile,
} from "./api";

describe("stageSubmitWorkFile", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("posts staged file requests to the session-scoped staged-files route", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          fileName: "ui.png",
          mediaType: "image/png",
          stagedFileRef: "/tmp/submit-work-stage/ui.png",
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 201,
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      stageSubmitWorkFile(
        {
          contentBase64: "cG5nLWJ5dGVz",
          fileName: "ui.png",
          itemType: "image",
          mediaType: "image/png",
        },
        { sessionID: "session-beta" },
      ),
    ).resolves.toEqual({
      fileName: "ui.png",
      mediaType: "image/png",
      stagedFileRef: "/tmp/submit-work-stage/ui.png",
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions/session-beta/work/staged-files",
      expect.objectContaining({
        body: JSON.stringify({
          contentBase64: "cG5nLWJ5dGVz",
          fileName: "ui.png",
          itemType: "image",
          mediaType: "image/png",
        }),
        method: "POST",
      }),
    );
  });

  it("surfaces structured staged-file errors", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            code: "BAD_REQUEST",
            family: "BAD_REQUEST",
            message: "mediaType must start with image/ for image items",
          }),
          {
            headers: {
              "Content-Type": "application/json",
            },
            status: 400,
            statusText: "Bad Request",
          },
        ),
      ),
    );

    await expect(
      stageSubmitWorkFile({
        contentBase64: "cG5nLWJ5dGVz",
        fileName: "ui.png",
        itemType: "image",
        mediaType: "application/pdf",
      }),
    ).rejects.toEqual(
      new StageSubmitWorkFileAPIError({
        code: "BAD_REQUEST",
        message: "mediaType must start with image/ for image items",
        status: 400,
        statusText: "Bad Request",
      }),
    );
  });
});

describe("isStageSubmitWorkFileAPIError", () => {
  it("returns true for staged submit-work file API errors", () => {
    const error = new StageSubmitWorkFileAPIError({
      code: "BAD_REQUEST",
      message: "Invalid staged file.",
      status: 400,
      statusText: "Bad Request",
    });
    expect(isStageSubmitWorkFileAPIError(error)).toBe(true);
  });

  it("returns false for unrelated errors", () => {
    expect(isStageSubmitWorkFileAPIError(new Error("offline"))).toBe(false);
    expect(isStageSubmitWorkFileAPIError(null)).toBe(false);
  });
});
