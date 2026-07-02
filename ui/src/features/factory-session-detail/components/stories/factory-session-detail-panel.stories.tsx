import {
  FailedBridgedChildInspection as failedBridgedChildInspectionStory,
  LiveProviderAdjacentSurfacesRegression as liveProviderAdjacentSurfacesRegressionStory,
  LiveProviderDispatchDetailUnavailable as liveProviderDispatchDetailUnavailableStory,
  LiveProviderSuccessInspection as liveProviderSuccessInspectionStory,
} from "./factory-session-detail-panel.live-provider-story-definitions.stories.shared";
import {
  DispatchDrilldownStates as dispatchDrilldownStatesStory,
} from "../../lib/factory-session-detail-panel.story-definitions.stories.shared";
import { ArtifactDrilldown as artifactDrilldownStory } from "../../lib/factory-session-detail-panel.artifact-story-definitions.stories.shared";
import {
  DurableReplayDisclosure as durableReplayDisclosureStory,
  DurableReplayDisclosureAwaitingApproval as durableReplayDisclosureAwaitingApprovalStory,
  DurableReplayDisclosureAwaitingApprovalMobile as durableReplayDisclosureAwaitingApprovalMobileStory,
  DurableReplayDisclosureUnavailable as durableReplayDisclosureUnavailableStory,
  DurableReplayDisclosureWarning as durableReplayDisclosureWarningStory,
  SessionError as sessionErrorStory,
  SessionUnavailable as sessionUnavailableStory,
} from "../../lib/factory-session-detail-panel.replay-story-definitions.stories.shared";
import { FactorySessionDetailPanel } from "../factory-session-detail-panel";

export default {
  title: "you-agent-factory/Current Selection/Factory Session Detail Panel",
  component: FactorySessionDetailPanel,
};

export const DispatchDrilldownStates = { ...dispatchDrilldownStatesStory };
export const LiveProviderSuccessInspection = {
  ...liveProviderSuccessInspectionStory,
};
export const FailedBridgedChildInspection = {
  ...failedBridgedChildInspectionStory,
};
export const LiveProviderAdjacentSurfacesRegression = {
  ...liveProviderAdjacentSurfacesRegressionStory,
};
export const LiveProviderDispatchDetailUnavailable = {
  ...liveProviderDispatchDetailUnavailableStory,
};
export const ArtifactDrilldown = { ...artifactDrilldownStory };
export const DurableReplayDisclosure = { ...durableReplayDisclosureStory };
export const DurableReplayDisclosureAwaitingApproval = {
  ...durableReplayDisclosureAwaitingApprovalStory,
};
export const DurableReplayDisclosureAwaitingApprovalMobile = {
  ...durableReplayDisclosureAwaitingApprovalMobileStory,
};
export const DurableReplayDisclosureUnavailable = {
  ...durableReplayDisclosureUnavailableStory,
};
export const DurableReplayDisclosureWarning = {
  ...durableReplayDisclosureWarningStory,
};
export const SessionError = { ...sessionErrorStory };
export const SessionUnavailable = { ...sessionUnavailableStory };
