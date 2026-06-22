import {
  type FactorySessionArtifactDetail,
  FactorySessionsAPIError,
} from "../../../api/factory-sessions";
import {
  hasUsableArtifactDownload,
  normalizeFactorySessionArtifactDrilldown,
  normalizeFactorySessionArtifactDrilldownLoadFailure,
} from "./factory-session-artifact-drilldown";
import artifactFixture from "./fixtures/durable-artifact-drilldown.fixture.json";

describe("normalizeFactorySessionArtifactDrilldown", () => {
  it("normalizes previewable durable artifacts into inline preview content", () => {
    const normalized = normalizeFactorySessionArtifactDrilldown(
      artifactFixture.previewable as FactorySessionArtifactDetail,
    );

    expect(normalized).toMatchObject({
      artifactId: "art-js-success-001",
      kind: "FINAL_RESULT",
      label: "Docs refresh output",
      preview: {
        kind: "inline",
      },
      sessionId: "dur-sess-js-success-002",
      visibility: "PUBLIC",
    });
    expect(normalized.preview.kind).toBe("inline");
    if (normalized.preview.kind !== "inline") {
      throw new Error("Expected inline preview content.");
    }
    expect(normalized.preview.content).toEqual([
      {
        text: "Documentation refresh complete.",
        type: "text",
      },
    ]);
  });

  it("normalizes content-ref durable artifacts into download metadata", () => {
    const normalized = normalizeFactorySessionArtifactDrilldown(
      artifactFixture.downloadOnly as FactorySessionArtifactDetail,
    );

    expect(normalized).toMatchObject({
      artifactId: "art-js-pause-001",
      kind: "FINDING",
      label: "Approval draft",
      preview: {
        kind: "download",
      },
      sessionId: "dur-sess-js-paused-001",
      visibility: "PUBLIC",
    });
    expect(normalized.preview.kind).toBe("download");
    if (normalized.preview.kind !== "download") {
      throw new Error("Expected download-only preview metadata.");
    }
    expect(normalized.preview.contentRef).toEqual({
      href: "/factory-sessions/dur-sess-js-paused-001/artifacts/art-js-pause-001",
      method: "GET",
    });
  });

  it("treats self-referential artifact detail refs as unavailable download paths", () => {
    const normalized = normalizeFactorySessionArtifactDrilldown(
      artifactFixture.downloadOnly as FactorySessionArtifactDetail,
    );

    expect(hasUsableArtifactDownload(normalized)).toBe(false);
  });

  it("allows download actions when the retrieval ref points to a distinct payload path", () => {
    expect(
      hasUsableArtifactDownload({
        artifactId: "art-js-success-001",
        preview: {
          kind: "download",
          contentRef: {
            href: "/factory-sessions/dur-sess-js-success-002/artifacts/art-js-success-001/content",
            method: "GET",
          },
        },
        sessionId: "dur-sess-js-success-002",
      }),
    ).toBe(true);
  });

  it("treats inline previews as non-downloadable drilldowns", () => {
    expect(
      hasUsableArtifactDownload({
        artifactId: "art-js-inline-001",
        preview: {
          content: [{ text: "inline artifact", type: "text" }],
          kind: "inline",
        },
        sessionId: "dur-sess-js-inline-001",
      }),
    ).toBe(false);
  });

  it("normalizes self-referential download refs that only differ by query, hash, or trailing slash", () => {
    expect(
      hasUsableArtifactDownload({
        artifactId: "art-js-success-001",
        preview: {
          kind: "download",
          contentRef: {
            href: "/factory-sessions/dur-sess-js-success-002/artifacts/art-js-success-001/?download=1#content",
            method: "GET",
          },
        },
        sessionId: "dur-sess-js-success-002",
      }),
    ).toBe(false);
  });
});

describe("normalizeFactorySessionArtifactDrilldownLoadFailure", () => {
  it("maps 404 artifact errors to a typed not-found failure", () => {
    const failure = normalizeFactorySessionArtifactDrilldownLoadFailure(
      new FactorySessionsAPIError("Artifact missing.", {
        code: "INTERNAL_ERROR",
        status: 404,
      }),
    );

    expect(failure).toEqual({
      kind: "not-found",
      message: "Artifact missing.",
    });
  });

  it("maps network errors to a typed network failure", () => {
    const failure = normalizeFactorySessionArtifactDrilldownLoadFailure(
      new FactorySessionsAPIError(
        "The dashboard could not reach the factory sessions API.",
        {
          code: "NETWORK_ERROR",
        },
      ),
    );

    expect(failure).toEqual({
      kind: "network",
      message: "The dashboard could not reach the factory sessions API.",
    });
  });

  it("maps invalid response errors to a typed invalid-response failure", () => {
    const failure = normalizeFactorySessionArtifactDrilldownLoadFailure(
      new FactorySessionsAPIError(
        "The factory sessions API returned an invalid response.",
        {
          code: "INTERNAL_ERROR",
          status: 200,
        },
      ),
    );

    expect(failure).toEqual({
      kind: "invalid-response",
      message: "The factory sessions API returned an invalid response.",
    });
  });

  it("maps non-API errors to an unknown failure with the original error message", () => {
    const failure = normalizeFactorySessionArtifactDrilldownLoadFailure(
      new Error("Artifact normalization crashed."),
    );

    expect(failure).toEqual({
      kind: "unknown",
      message: "Artifact normalization crashed.",
    });
  });
});
