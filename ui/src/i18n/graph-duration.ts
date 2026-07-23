type GraphDurationUnit =
  | "hundredQuintillionYear"
  | "quintillionYear"
  | "hundredQuadrillionYear"
  | "quadrillionYear"
  | "hundredTrillionYear"
  | "trillionYear"
  | "tenBillionYear"
  | "hundredMillionYear"
  | "myriadYear"
  | "millennium"
  | "century"
  | "year"
  | "week"
  | "day"
  | "hour"
  | "minute"
  | "second";

const GRAPH_DURATION_LABELS: Record<
  string,
  Record<GraphDurationUnit, string>
> = {
  en: {
    hundredQuintillionYear: "Y",
    quintillionYear: "Z",
    hundredQuadrillionYear: "E",
    quadrillionYear: "Q",
    hundredTrillionYear: "T",
    trillionYear: "B",
    tenBillionYear: "H",
    hundredMillionYear: "M",
    myriadYear: "q",
    millennium: "ky",
    century: "c",
    year: "y",
    week: "w",
    day: "d",
    hour: "h",
    minute: "m",
    second: "s",
  },
  "zh-CN": {
    hundredQuintillionYear: "垓",
    quintillionYear: "百京",
    hundredQuadrillionYear: "京",
    quadrillionYear: "百兆",
    hundredTrillionYear: "兆",
    trillionYear: "百亿",
    tenBillionYear: "亿",
    hundredMillionYear: "百万",
    myriadYear: "万",
    millennium: "千年",
    century: "世纪",
    year: "年",
    week: "周",
    day: "天",
    hour: "时",
    minute: "分",
    second: "秒",
  },
  ja: {
    hundredQuintillionYear: "垓",
    quintillionYear: "百京",
    hundredQuadrillionYear: "京",
    quadrillionYear: "百兆",
    hundredTrillionYear: "兆",
    trillionYear: "百億",
    tenBillionYear: "億",
    hundredMillionYear: "百万",
    myriadYear: "万",
    millennium: "千年",
    century: "世紀",
    year: "年",
    week: "週",
    day: "日",
    hour: "時",
    minute: "分",
    second: "秒",
  },
  ko: {
    hundredQuintillionYear: "해",
    quintillionYear: "백경",
    hundredQuadrillionYear: "경",
    quadrillionYear: "백조",
    hundredTrillionYear: "조",
    trillionYear: "백억",
    tenBillionYear: "억",
    hundredMillionYear: "백만",
    myriadYear: "만",
    millennium: "천년",
    century: "세기",
    year: "년",
    week: "주",
    day: "일",
    hour: "시",
    minute: "분",
    second: "초",
  },
};

const GRAPH_DURATION_UNITS: ReadonlyArray<{
  millis: number;
  unit: GraphDurationUnit;
}> = [
  {
    unit: "hundredQuintillionYear",
    millis: 100_000_000_000_000_000_000 * 365 * 24 * 60 * 60 * 1000,
  },
  {
    unit: "quintillionYear",
    millis: 1_000_000_000_000_000_000 * 365 * 24 * 60 * 60 * 1000,
  },
  {
    unit: "hundredQuadrillionYear",
    millis: 100_000_000_000_000_000 * 365 * 24 * 60 * 60 * 1000,
  },
  {
    unit: "quadrillionYear",
    millis: 1_000_000_000_000_000 * 365 * 24 * 60 * 60 * 1000,
  },
  {
    unit: "hundredTrillionYear",
    millis: 100_000_000_000_000 * 365 * 24 * 60 * 60 * 1000,
  },
  {
    unit: "trillionYear",
    millis: 1_000_000_000_000 * 365 * 24 * 60 * 60 * 1000,
  },
  { unit: "tenBillionYear", millis: 100_000_000 * 365 * 24 * 60 * 60 * 1000 },
  { unit: "hundredMillionYear", millis: 1_000_000 * 365 * 24 * 60 * 60 * 1000 },
  { unit: "myriadYear", millis: 10_000 * 365 * 24 * 60 * 60 * 1000 },
  { unit: "millennium", millis: 1000 * 365 * 24 * 60 * 60 * 1000 },
  { unit: "century", millis: 100 * 365 * 24 * 60 * 60 * 1000 },
  { unit: "year", millis: 365 * 24 * 60 * 60 * 1000 },
  { unit: "week", millis: 7 * 24 * 60 * 60 * 1000 },
  { unit: "day", millis: 24 * 60 * 60 * 1000 },
  { unit: "hour", millis: 60 * 60 * 1000 },
  { unit: "minute", millis: 60 * 1000 },
  { unit: "second", millis: 1000 },
];

const BASE_GRAPH_DURATION_UNITS = GRAPH_DURATION_UNITS.filter(
  ({ unit }) => unit === "hour" || unit === "minute" || unit === "second",
);

export function formatGraphDurationToken(
  durationMillis: number,
  locale: string,
): string {
  const baseGraphUnit = getBaseGraphDurationUnit(durationMillis);
  const fittingGraphUnit = getFittingGraphDurationUnit(
    durationMillis,
    baseGraphUnit,
    locale,
  );
  const graphValue = Math.floor(durationMillis / fittingGraphUnit.millis);

  return `${new Intl.NumberFormat(locale).format(graphValue)}${getGraphDurationLabel(fittingGraphUnit.unit, locale)}`;
}

function getBaseGraphDurationUnit(durationMillis: number): {
  millis: number;
  unit: GraphDurationUnit;
} {
  for (const graphUnit of BASE_GRAPH_DURATION_UNITS) {
    if (durationMillis >= graphUnit.millis) {
      return graphUnit;
    }
  }

  return BASE_GRAPH_DURATION_UNITS.at(-1) ?? { unit: "second", millis: 1000 };
}

function getFittingGraphDurationUnit(
  durationMillis: number,
  baseUnit: { millis: number; unit: GraphDurationUnit },
  locale: string,
): { millis: number; unit: GraphDurationUnit } {
  const baseUnitIndex = GRAPH_DURATION_UNITS.findIndex(
    ({ unit }) => unit === baseUnit.unit,
  );

  for (let index = baseUnitIndex; index >= 0; index -= 1) {
    const graphUnit = GRAPH_DURATION_UNITS[index];
    if (!graphUnit) {
      continue;
    }
    const graphValue = Math.floor(durationMillis / graphUnit.millis);
    if (graphValue <= getMaxGraphDurationValue(graphUnit.unit, locale)) {
      return graphUnit;
    }
  }

  return GRAPH_DURATION_UNITS[0];
}

function getMaxGraphDurationValue(
  unit: GraphDurationUnit,
  locale: string,
): number {
  const maxDigits = Math.max(1, 3 - getGraphDurationLabel(unit, locale).length);
  return 10 ** maxDigits - 1;
}

function getGraphDurationLabel(
  unit: GraphDurationUnit,
  locale: string,
): string {
  return (
    GRAPH_DURATION_LABELS[locale]?.[unit] ?? GRAPH_DURATION_LABELS.en[unit]
  );
}
