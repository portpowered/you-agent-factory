import "./guarded-suite-console.setup";

import { configure } from "@testing-library/react";

Object.assign(globalThis, {
  IS_REACT_ACT_ENVIRONMENT: true,
});

configure({
  asyncUtilTimeout: 10_000,
});
