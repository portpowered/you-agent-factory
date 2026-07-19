import type { FactoryRecordingTopologyReplayMessages } from "../factory-recording-topology-replay";

export function createGermanRecordingMessages(): FactoryRecordingTopologyReplayMessages {
  const kindNames = {
    resource: "Ressource",
    worker: "Worker",
    "work-state": "Arbeitsstatus",
    "work-type": "Arbeitsart",
    workstation: "Arbeitsstation",
  } as const;
  const workCount = (count: string, label: string) =>
    `${count} ${label}${count === "1" ? "" : "e"}`;
  return {
    progress: {
      categories: {
        active: {
          plural: (count) => workCount(count, "aktive Aufträg"),
          singular: (count) => workCount(count, "aktiver Auftrag"),
        },
        completed: {
          plural: (count) => workCount(count, "abgeschlossene Aufträg"),
          singular: (count) => workCount(count, "abgeschlossener Auftrag"),
        },
        failed: {
          plural: (count) => workCount(count, "fehlgeschlagene Aufträg"),
          singular: (count) => workCount(count, "fehlgeschlagener Auftrag"),
        },
        queued: {
          plural: (count) => `${count} wartende Aufträge`,
          singular: (count) => `${count} wartender Auftrag`,
        },
        unclassified: {
          plural: (count) => `${count} sonstige Aufträge`,
          singular: (count) => `${count} sonstiger Auftrag`,
        },
      },
      empty: "Keine Aufträge wurden aufgezeichnet.",
      regionLabel: "Aufgezeichneter Arbeitsfortschritt",
      title: "Arbeitsfortschritt",
      total: (count) =>
        `${count} ${count === "1" ? "Auftrag" : "Aufträge"} insgesamt`,
    },
    regionLabel: "Aufgezeichnete Fabrikwiedergabe",
    selectedTick: (tick) => `Ausgewählter logischer Schritt ${tick}`,
    timeline: {
      alreadyFollowingLatest: "Die aktuelle Aufzeichnung wird bereits verfolgt",
      currentMode: "Aktuelle Aufzeichnung verfolgen",
      disabled: "Aufzeichnungswiedergabe ist deaktiviert",
      followLatest: "Zum neuesten Stand",
      historyMode: "Aufzeichnungsverlauf prüfen",
      position: (selected, latest) => `Schritt ${selected} von ${latest}`,
      regionLabel: "Aufzeichnungszeitachse",
      sliderLabel: "Aufzeichnungsschritt auswählen",
      title: "Aufzeichnungszeitachse",
      unavailable: "Aufzeichnungszeitachse nicht verfügbar",
    },
    topology: {
      activeDispatches: (count) =>
        `${count} aktive ${count === 1 ? "Ausführung" : "Ausführungen"}`,
      annotationsHidden: "Anmerkungen anzeigen",
      annotationsVisible: "Anmerkungen ausblenden",
      empty: "Für diesen Schritt ist keine Fabriktopologie verfügbar.",
      failed: "Die Fabriktopologie konnte nicht angezeigt werden.",
      inactiveDispatches: "Keine aktive Ausführung",
      imageFailed: "Das Anmerkungsbild konnte nicht angezeigt werden.",
      imageLoading: "Anmerkungsbild wird geladen.",
      legendActiveRoute: "Aktive Route",
      legendInactiveRoute: "Inaktive Route",
      legendLabel: "Topologielegende",
      loading: "Fabriktopologie wird geladen.",
      nodeLabel: (kind, label) => `${kindNames[kind]}: ${label}`,
      regionLabel: "Aufgezeichnete Fabriktopologie",
      resourceOccupancy: (occupied, capacity) =>
        `${occupied} von ${capacity} Kapazitäten belegt`,
      resourceOccupancyUnavailable: "Belegung nicht verfügbar",
      retry: "Erneut versuchen",
      selectedNode: "Ausgewählt",
      viewportControlsLabel: "Steuerelemente für den Topologieausschnitt",
      workStateCount: (count) => `${count} Aufträge in diesem Status`,
      workStateCountUnavailable: "Auftragszahl nicht verfügbar",
    },
    validationFailed: "Die Fabrikaufzeichnung konnte nicht validiert werden.",
  };
}
