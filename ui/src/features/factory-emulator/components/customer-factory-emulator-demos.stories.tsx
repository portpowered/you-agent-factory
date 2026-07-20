import type { FactoryEmulatorScenario } from "@you-agent-factory/factory-emulator";

import { customerFactoryEmulatorDemoFixtures } from "../lib/customer-demo-fixtures";
import { CustomerFactoryEmulatorDemos } from "./customer-factory-emulator-demos";

export default {
  title: "Agent Factory/Emulator/Customer Demos",
  component: CustomerFactoryEmulatorDemos,
  parameters: { layout: "fullscreen" },
};

export const Interactive = {
  render: () => <CustomerFactoryEmulatorDemos locale="en" />,
};

const invalidScenario = {
  ...customerFactoryEmulatorDemoFixtures.repeatReviewFailure.scenario,
  factory: { name: "invalid-customer-demo-factory" },
} satisfies FactoryEmulatorScenario;

export const SetupErrorIsolation = {
  render: () => (
    <CustomerFactoryEmulatorDemos
      fixtures={[
        customerFactoryEmulatorDemoFixtures.success,
        {
          ...customerFactoryEmulatorDemoFixtures.repeatReviewFailure,
          scenario: invalidScenario,
        },
      ]}
      locale="en"
    />
  ),
};
