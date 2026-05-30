import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface WorkContentPartTypeLabels {
  audio: string;
  binary: string;
  fallback: string;
  image: string;
  json: string;
  text: string;
  unknownType: (type: string) => string;
}

export interface WorkContentInspectMessages {
  empty: string;
  error: string;
  heading: string;
  loading: string;
  partTypeLabels: WorkContentPartTypeLabels;
  unavailable: string;
}

const workContentInspectMessagesByLocale = {
  en: {
    empty: "No work content items are available for this selection.",
    error: "Work content could not be loaded.",
    heading: "Work contents",
    loading: "Loading work content…",
    partTypeLabels: {
      audio: "Audio",
      binary: "Binary",
      fallback: "Content",
      image: "Image",
      json: "JSON",
      text: "Text",
      unknownType: (type) => type,
    },
    unavailable: "Work content is unavailable.",
  },
} satisfies LocalizedMessageCatalog<WorkContentInspectMessages>;

export function getWorkContentInspectMessages(
  locale?: string,
): WorkContentInspectMessages {
  return resolveLocalizedMessages(
    workContentInspectMessagesByLocale,
    locale,
  );
}
