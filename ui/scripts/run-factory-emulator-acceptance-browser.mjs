import { runStorybookCI } from "./run-storybook-ci.mjs";

await runStorybookCI({
  browserChecks: [
    ["run", "storybook:factory-emulator-adapter-check"],
    ["run", "storybook:customer-factory-emulator-demos-check"],
  ],
  includeInteractionSuite: false,
});
