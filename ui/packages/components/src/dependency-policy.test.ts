import { describe, expect, it } from "vitest";

import packageJson from "../package.json";

describe("youagentfactory/components dependency policy", () => {
  it("declares react and react-dom as peer dependencies", () => {
    expect(packageJson.peerDependencies?.react).toBeDefined();
    expect(packageJson.peerDependencies?.["react-dom"]).toBeDefined();
    expect(packageJson.dependencies?.react).toBeUndefined();
    expect(packageJson.dependencies?.["react-dom"]).toBeUndefined();
  });

  it("declares recharts and @xyflow/react as regular package dependencies", () => {
    expect(packageJson.dependencies?.recharts).toBeDefined();
    expect(packageJson.dependencies?.["@xyflow/react"]).toBeDefined();
    expect(packageJson.peerDependencies?.recharts).toBeUndefined();
    expect(packageJson.peerDependencies?.["@xyflow/react"]).toBeUndefined();
  });

  it("exposes scripts that typecheck without dashboard application modules", () => {
    expect(packageJson.scripts.typecheck).toContain("tsc");
    expect(packageJson.scripts["check:package-boundary"]).toBeDefined();
  });
});
