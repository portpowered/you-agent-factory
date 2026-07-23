import canonicalCliManifest from "../../../../../contracts/cli/commands.json" with {
  type: "json",
};
import { projectCliManifest } from "./cli-command-projection";
import { loadCliManifest } from "./cli-manifest-adapter";

function projectCanonicalManifest() {
  const loaded = loadCliManifest(canonicalCliManifest);
  expect(loaded.status).toBe("ready");
  if (loaded.status !== "ready") throw new Error("Expected ready manifest.");
  return projectCliManifest(loaded);
}

describe("CLI command navigation and root projections", () => {
  it("projects the root and deterministic nested navigation in manifest order", () => {
    const projection = projectCanonicalManifest();

    expect(projection.rootCommandId).toBe("you");
    expect(projection.navigation).toMatchObject({
      id: "you",
      path: "you",
      lifecycleState: "active",
      visibility: "visible",
    });
    expect(
      projection.navigation.children.slice(0, 3).map(({ id }) => id),
    ).toEqual(["you.config", "you.factory", "you.init"]);
    expect(
      projection.navigation.children
        .find(({ id }) => id === "you.factory")
        ?.children.map(({ id }) => id),
    ).toEqual([
      "you.factory.config",
      "you.factory.create",
      "you.factory.delete",
      "you.factory.list",
      "you.factory.query",
      "you.factory.replace-current",
      "you.factory.update",
    ]);
    expect(projection.commandOrder).toEqual(
      Object.keys(canonicalCliManifest.commands),
    );
  });

  it("projects root help, examples, lifecycle, and global inputs", () => {
    const projection = projectCanonicalManifest();
    const root = projection.commands.you;

    expect(root.help.title).toEqual({
      id: "you.title",
      canonicalEnglish: "Run and manage CPN-based workflow factories",
    });
    expect(root.examples).toContain("you docs agents");
    expect(root.lifecycle.state).toBe("active");
    expect(root.visibility).toBe("visible");
    expect(root.inheritedInputs).toEqual([]);
    expect(root.localInputs.map(({ id }) => id)).toContain("you.flag.verbose");
    expect(
      root.localInputs.find(({ id }) => id === "you.flag.verbose")?.source,
    ).toEqual({
      commandId: "you",
      inputId: "you.flag.verbose",
      scope: "persistent",
    });
  });
});

describe("CLI nested command projections", () => {
  it("projects nested help, arguments, inherited inputs, and relationships", () => {
    const projection = projectCanonicalManifest();
    const run = projection.commands["you.run"];

    expect(run.path).toBe("you run");
    expect(run.help.title.canonicalEnglish).toBe(
      "Load workflow and run the factory engine",
    );
    expect(run.examples[0]).toContain(
      "you run --work ./docs/examples/startup-work.json",
    );
    expect(run.localInputs[0]).toMatchObject({
      id: "you.run.arg.0",
      kind: "argument",
      cardinality: { minimum: 0, maximum: null },
    });

    const inheritedVerbose = run.inheritedInputs.filter(
      ({ manifestInput }) =>
        "long" in manifestInput && manifestInput.long === "verbose",
    );
    expect(inheritedVerbose).toHaveLength(1);
    expect(inheritedVerbose[0]).toMatchObject({
      id: "you.run.flag.verbose",
      inherited: true,
      source: {
        commandId: "you",
        inputId: "you.flag.verbose",
        scope: "persistent",
      },
    });
    const canonicalInheritedVerbose =
      canonicalCliManifest.commands["you.run"].flags["you.run.flag.verbose"];
    expect(inheritedVerbose[0]?.source.inputId).toBe(
      canonicalInheritedVerbose.inheritedFromInputId,
    );
    expect(
      run.effectiveInputs.filter(({ id }) => id === "you.run.flag.verbose"),
    ).toHaveLength(1);

    const selectorRelationship = run.relationships.find(
      ({ id }) => id === "you.run.rel.selectors",
    );
    expect(
      selectorRelationship?.participants.map(({ inputId }) => inputId),
    ).toEqual([
      "you.run.flag.dir",
      "you.run.flag.named",
      "you.run.flag.factory",
    ]);
    expect(selectorRelationship?.participants[0]?.input).toBe(
      run.effectiveInputs.find(({ id }) => id === "you.run.flag.dir"),
    );
  });

  it("can reproject the same canonical object without mutation or drift", () => {
    const before = JSON.stringify(canonicalCliManifest);
    const loaded = loadCliManifest(canonicalCliManifest);
    expect(loaded.status).toBe("ready");
    if (loaded.status !== "ready") throw new Error("Expected ready manifest.");

    const first = projectCliManifest(loaded);
    const second = projectCliManifest(loaded);

    expect(second).toEqual(first);
    expect(second).not.toBe(first);
    expect(first.commands.you.examples).toBe(
      canonicalCliManifest.commands.you.documentation.examples,
    );
    expect(
      first.commands["you.run"].inheritedInputs.find(
        ({ id }) => id === "you.run.flag.verbose",
      )?.manifestInput,
    ).toBe(
      canonicalCliManifest.commands["you.run"].flags["you.run.flag.verbose"],
    );
    expect(JSON.stringify(canonicalCliManifest)).toBe(before);
  });
});
