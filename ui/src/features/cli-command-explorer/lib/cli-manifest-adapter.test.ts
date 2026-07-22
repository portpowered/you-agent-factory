import canonicalCliManifest from "../../../../../contracts/cli/commands.json" with {
  type: "json",
};
import {
  loadCliManifest,
  loadingCliManifest,
  loadPublishedCliManifest,
} from "./cli-manifest-adapter";

function lifecycle(id: string) {
  return {
    formatVersion: "1.0.0",
    itemId: id,
    since: "1.0.0",
    state: "active",
  };
}

function documentation(id: string) {
  return {
    formatVersion: "1.0.0",
    itemId: id,
    documentation: {
      title: { id: `${id}.title`, canonicalEnglish: `Title for ${id}` },
      description: {
        id: `${id}.description`,
        canonicalEnglish: `Description for ${id}`,
      },
    },
    examples: [id],
    visibility: "public",
    sourceHash:
      "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  };
}

function flag(id: string, scope: "inherited" | "local" | "persistent") {
  return {
    id,
    long: "verbose",
    shorthand: "v",
    aliases: [],
    scope,
    valueType: "bool",
    required: false,
    default: "false",
    changedDefault: false,
    noOptionDefault: "true",
    repeatable: false,
    normalization: "",
    completion: "none",
    binding: "",
    visibility: "visible",
    lifecycle: lifecycle(id),
  };
}

function command(id: string, path: string) {
  return {
    id,
    name: path.split(" ").at(-1),
    path,
    aliases: [],
    documentation: documentation(id),
    lifecycle: lifecycle(id),
    visibility: "visible",
    runnable: true,
    usage: { line: path },
  };
}

function validManifest() {
  return {
    formatVersion: "1.0.0",
    rootPath: "you",
    commands: {
      you: {
        ...command("you", "you"),
        flags: { "you.flag.verbose": flag("you.flag.verbose", "persistent") },
      },
      "you.run": {
        ...command("you.run", "you run"),
        flags: {
          "you.run.flag.verbose": flag("you.run.flag.verbose", "inherited"),
          "you.run.flag.work": {
            ...flag("you.run.flag.work", "local"),
            long: "work",
            shorthand: "w",
            valueType: "string",
            default: "",
            noOptionDefault: "",
          },
        },
        arguments: {
          "you.run.arg.prompt": {
            id: "you.run.arg.prompt",
            name: "prompt",
            position: 0,
            kind: "positional",
            valueType: "string",
            required: false,
            minCardinality: 0,
            maxCardinality: 1,
            variadic: false,
            enum: [],
            pattern: "",
            completion: "none",
            channels: ["cli"],
            doubleDash: "terminates-flags",
          },
        },
        relationships: {
          "you.run.rel.choice": {
            id: "you.run.rel.choice",
            kind: "mutually-exclusive",
            participants: [
              { type: "argument", id: "you.run.arg.prompt" },
              { type: "flag", id: "you.run.flag.work" },
            ],
          },
        },
      },
    },
  };
}

function diagnosticCodes(input: unknown) {
  const result = loadCliManifest(input);
  expect(result.status).toBe("invalid-contract");
  return result.status === "invalid-contract"
    ? result.diagnostics.map((diagnostic) => diagnostic.code)
    : [];
}

describe("CLI manifest adapter", () => {
  it("exposes an explicit loading state", () => {
    expect(loadingCliManifest()).toEqual({ status: "loading" });
  });

  it("accepts the canonical rich manifest without copying or mutating it", () => {
    const before = JSON.stringify(canonicalCliManifest);
    const result = loadCliManifest(canonicalCliManifest);

    expect(result.status).toBe("ready");
    if (result.status !== "ready") return;
    expect(result.manifest).toBe(canonicalCliManifest);
    expect(result.manifest.commands.you.path).toBe("you");
    expect(
      result.manifest.commands["you.run"].flags?.["you.run.flag.verbose"]
        ?.scope,
    ).toBe("inherited");
    expect(JSON.stringify(canonicalCliManifest)).toBe(before);
  });

  it("returns empty only for a valid supported manifest with no commands", () => {
    const manifest = { formatVersion: "1.0.0", rootPath: "you", commands: {} };
    expect(loadCliManifest(manifest)).toEqual({ status: "empty", manifest });
    expect(
      diagnosticCodes({ ...manifest, rootPath: "you  invalid" }),
    ).toContain("invalid_hierarchy");
  });

  it("accepts a representative root and nested command graph", () => {
    expect(loadCliManifest(validManifest())).toMatchObject({ status: "ready" });
  });

  it("rejects unknown versions without creating a partial projection", () => {
    expect(
      loadCliManifest({ ...validManifest(), formatVersion: "2.0.0" }),
    ).toEqual({
      status: "unsupported-version",
      receivedVersion: "2.0.0",
      supportedVersions: ["1.0.0"],
    });
  });

  it("loads the supported rich graph from the published package artifact", () => {
    const result = loadPublishedCliManifest();
    expect(result).toMatchObject({ status: "ready" });
    if (result.status !== "ready") return;
    expect(result.manifest.commands.you.path).toBe("you");
    expect(result.manifest.commands["you.run"].path).toBe("you run");
  });

  it("returns deterministic actionable structural diagnostics", () => {
    const result = loadCliManifest({
      formatVersion: "1.0.0",
      rootPath: 4,
      commands: [],
    });
    expect(result).toEqual({
      status: "invalid-contract",
      diagnostics: [
        {
          code: "invalid_type",
          path: ["commands"],
          message:
            "Expected commands to satisfy the CLI manifest contract: must be object.",
        },
        {
          code: "invalid_type",
          path: ["rootPath"],
          message:
            "Expected rootPath to satisfy the CLI manifest contract: must be string.",
        },
      ],
    });
  });

  it("rejects unstable and duplicate command or input identities", () => {
    const unstable = validManifest();
    unstable.commands["you.run"].id = "You Run";
    expect(diagnosticCodes(unstable)).toContain("unstable_id");

    const duplicate = validManifest();
    duplicate.commands.you.flags = { you: flag("you", "local") };
    expect(diagnosticCodes(duplicate)).toContain("duplicate_id");
  });

  it("rejects invalid roots and missing parent command paths", () => {
    const invalidRoot = { ...validManifest(), rootPath: "other" };
    expect(diagnosticCodes(invalidRoot)).toContain("invalid_hierarchy");

    const missingParent = validManifest();
    missingParent.commands["you.run"].path = "you factory run";
    expect(diagnosticCodes(missingParent)).toContain("invalid_hierarchy");
  });

  it("rejects inherited flags without a matching persistent ancestor", () => {
    const manifest = validManifest();
    manifest.commands.you.flags["you.flag.verbose"].default = "true";
    expect(diagnosticCodes(manifest)).toContain("contradictory_inheritance");
  });

  it("rejects impossible argument and repeatable-flag cardinality", () => {
    const manifest = validManifest();
    manifest.commands["you.run"].arguments["you.run.arg.prompt"].variadic =
      true;
    manifest.commands["you.run"].flags["you.run.flag.work"].repeatable = true;
    expect(diagnosticCodes(manifest)).toEqual(
      expect.arrayContaining(["invalid_cardinality", "invalid_cardinality"]),
    );
  });

  it("rejects relationships with missing or invalid participants", () => {
    const manifest = validManifest();
    manifest.commands["you.run"].relationships[
      "you.run.rel.choice"
    ].participants = [
      { type: "flag", id: "you.run.flag.missing" },
      { type: "argument", id: "you.run.arg.prompt" },
    ];
    const codes = diagnosticCodes(manifest);
    expect(codes).toContain("invalid_reference");
    expect(codes.filter((code) => code === "invalid_reference")).toHaveLength(
      1,
    );
  });
});
