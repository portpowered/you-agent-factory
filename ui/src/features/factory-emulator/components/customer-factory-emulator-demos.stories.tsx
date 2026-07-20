import type { FactoryEmulatorScenario } from "@you-agent-factory/factory-emulator";
import { useState } from "react";

import { Button } from "../../../components/ui";
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

function LifecycleIsolationStory() {
  const [showFailureDemo, setShowFailureDemo] = useState(true);
  return (
    <div className="grid gap-4 p-4">
      <Button
        onClick={() => setShowFailureDemo((visible) => !visible)}
        type="button"
        variant="outline"
      >
        {showFailureDemo ? "Unmount failure demo" : "Remount failure demo"}
      </Button>
      <CustomerFactoryEmulatorDemos
        fixtures={[
          customerFactoryEmulatorDemoFixtures.success,
          ...(showFailureDemo
            ? [customerFactoryEmulatorDemoFixtures.repeatReviewFailure]
            : []),
        ]}
        locale="en"
      />
    </div>
  );
}

export const LifecycleIsolation = {
  render: () => <LifecycleIsolationStory />,
};
