import { type ReactNode, useEffect } from "react";

import type {
  DashboardSnapshot,
  DashboardTrace,
  DashboardWorkstationRequest,
} from "../api/dashboard";
import type { FactoryEvent } from "../api/events";
import { useFactoryTimelineStore } from "../features/timeline/state/factoryTimelineStore";
import { AppLocaleProvider } from "../i18n";
import { AppColorPaletteProvider } from "../theme";
import type { buildAppShellStreamIdentity } from "./app-shell-session-preflight-test-utils";
import {
  seedTimelineSnapshot,
  seedTimelineSnapshots,
} from "./app-shell-timeline-seed-utils";

interface AppShellTimelineSeederProps {
  identity: ReturnType<typeof buildAppShellStreamIdentity>;
  snapshot: DashboardSnapshot;
  timelineEvents?: FactoryEvent[];
  timelineSnapshots?: DashboardSnapshot[];
  traceFixtures: Record<string, DashboardTrace>;
  workstationRequestsByDispatchID: Record<string, DashboardWorkstationRequest>;
}

interface AppShellSeededAppProps extends AppShellTimelineSeederProps {
  browserLanguage?: string | null;
  browserLanguages?: readonly string[] | null;
  initialLocale?: string | null;
  locationSearch?: string | null;
  seedTimelineFromSnapshot: boolean;
  children: ReactNode;
}

export function AppShellSeededApp({
  browserLanguage,
  browserLanguages,
  initialLocale,
  locationSearch,
  seedTimelineFromSnapshot,
  children,
  ...timelineProps
}: AppShellSeededAppProps) {
  return (
    <>
      <AppColorPaletteProvider>
        <AppLocaleProvider
          browserLanguage={browserLanguage}
          browserLanguages={browserLanguages}
          initialLocale={initialLocale}
          locationSearch={locationSearch}
        >
          {children}
        </AppLocaleProvider>
      </AppColorPaletteProvider>
      {timelineProps.timelineEvents ||
      timelineProps.timelineSnapshots ||
      seedTimelineFromSnapshot ? (
        <AppShellTimelineSeeder {...timelineProps} />
      ) : null}
    </>
  );
}

function AppShellTimelineSeeder({
  identity,
  snapshot,
  timelineEvents,
  timelineSnapshots,
  traceFixtures,
  workstationRequestsByDispatchID,
}: AppShellTimelineSeederProps) {
  useEffect(() => {
    const timeline = useFactoryTimelineStore.getState();
    timeline.activateEntry(identity);
    if (timelineEvents) {
      timeline.replaceEvents(timelineEvents);
      return;
    }
    if (timelineSnapshots) {
      seedTimelineSnapshots(timelineSnapshots);
      return;
    }
    seedTimelineSnapshot(
      snapshot,
      traceFixtures,
      workstationRequestsByDispatchID,
    );
  }, [
    identity,
    snapshot,
    timelineEvents,
    timelineSnapshots,
    traceFixtures,
    workstationRequestsByDispatchID,
  ]);

  return null;
}
