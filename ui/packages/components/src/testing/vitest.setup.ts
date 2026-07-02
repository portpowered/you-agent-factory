import "@testing-library/jest-dom/vitest";

import { cleanup, configure } from "@testing-library/react";
import { toHaveNoViolations } from "jest-axe";
import { afterEach, beforeAll, expect } from "vitest";

import { injectCompiledPackageTokenStyles } from "../styles/compile-package-token-styles";

expect.extend(toHaveNoViolations);

Object.assign(globalThis, {
  IS_REACT_ACT_ENVIRONMENT: true,
});

configure({
  asyncUtilTimeout: 10_000,
});

beforeAll(async () => {
  if (typeof document !== "undefined") {
    await injectCompiledPackageTokenStyles(document);
  }
});

afterEach(() => {
  cleanup();
});
