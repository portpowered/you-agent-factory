import { readFileSync, writeFileSync } from "node:fs";

function normalizeSourcePath(sourcePath) {
  return sourcePath.replaceAll("\\", "/");
}

function parseRecord(lines) {
  const record = {
    branches: new Map(),
    functions: new Map(),
    lines: new Map(),
    sourcePath: "",
  };

  for (const line of lines) {
    if (line.startsWith("SF:")) {
      record.sourcePath = normalizeSourcePath(line.slice(3));
      continue;
    }
    if (line.startsWith("FN:")) {
      const [lineNumber, ...nameParts] = line.slice(3).split(",");
      const name = nameParts.join(",");
      record.functions.set(name, {
        line: Number(lineNumber),
        name,
      });
      continue;
    }
    if (line.startsWith("FNDA:")) {
      const [hitCount, ...nameParts] = line.slice(5).split(",");
      const name = nameParts.join(",");
      const functionData = record.functions.get(name) ?? {
        line: 0,
        name,
      };
      functionData.hitCount = Number(hitCount);
      record.functions.set(name, functionData);
      continue;
    }
    if (line.startsWith("DA:")) {
      const [lineNumber, hitCount, checksum] = line.slice(3).split(",");
      record.lines.set(Number(lineNumber), {
        checksum,
        hitCount: Number(hitCount),
        line: Number(lineNumber),
      });
      continue;
    }
    if (line.startsWith("BRDA:")) {
      const [lineNumber, block, branch, taken] = line.slice(5).split(",");
      record.branches.set(`${lineNumber},${block},${branch}`, {
        block,
        branch,
        line: Number(lineNumber),
        taken: taken === "-" ? 0 : Number(taken),
      });
    }
  }

  return record;
}

function parseLcovRecords(report) {
  const records = [];
  let lines = [];

  for (const line of report.split(/\r?\n/)) {
    if (line === "end_of_record") {
      if (lines.length > 0) {
        records.push(parseRecord(lines));
      }
      lines = [];
      continue;
    }
    if (line !== "") {
      lines.push(line);
    }
  }

  if (lines.length > 0) {
    records.push(parseRecord(lines));
  }

  return records;
}

function mergeCountedRecord(target, source) {
  for (const [name, functionData] of source.functions) {
    const existing = target.functions.get(name);
    if (existing === undefined) {
      target.functions.set(name, { ...functionData });
    } else {
      existing.hitCount += functionData.hitCount ?? 0;
    }
  }

  for (const [lineNumber, lineData] of source.lines) {
    const existing = target.lines.get(lineNumber);
    if (existing === undefined) {
      target.lines.set(lineNumber, { ...lineData });
    } else {
      existing.hitCount += lineData.hitCount;
    }
  }

  for (const [key, branchData] of source.branches) {
    const existing = target.branches.get(key);
    if (existing === undefined) {
      target.branches.set(key, { ...branchData });
    } else {
      existing.taken += branchData.taken;
    }
  }
}

function serializeRecord(record) {
  const lines = ["TN:", `SF:${record.sourcePath}`];

  for (const functionData of record.functions.values()) {
    lines.push(`FN:${functionData.line},${functionData.name}`);
  }
  for (const functionData of record.functions.values()) {
    lines.push(`FNDA:${functionData.hitCount ?? 0},${functionData.name}`);
  }
  lines.push(`FNF:${record.functions.size}`);
  lines.push(
    `FNH:${[...record.functions.values()].filter((item) => (item.hitCount ?? 0) > 0).length}`,
  );

  for (const branchData of record.branches.values()) {
    lines.push(
      `BRDA:${branchData.line},${branchData.block},${branchData.branch},${branchData.taken}`,
    );
  }
  lines.push(`BRF:${record.branches.size}`);
  lines.push(
    `BRH:${[...record.branches.values()].filter((item) => item.taken > 0).length}`,
  );

  for (const lineData of record.lines.values()) {
    const checksum = lineData.checksum ? `,${lineData.checksum}` : "";
    lines.push(`DA:${lineData.line},${lineData.hitCount}${checksum}`);
  }
  lines.push(`LF:${record.lines.size}`);
  lines.push(
    `LH:${[...record.lines.values()].filter((item) => item.hitCount > 0).length}`,
  );
  lines.push("end_of_record");
  return lines.join("\n");
}

export function isCoverageSourcePath(sourcePath) {
  return !/(?:\.bun\.)?\b(?:test|spec)\.[cm]?[jt]sx?$/.test(sourcePath);
}

export function mergeLcovReports(baseReport, supplementalReport) {
  const merged = new Map();

  for (const record of parseLcovRecords(baseReport)) {
    merged.set(record.sourcePath, record);
  }
  for (const record of parseLcovRecords(supplementalReport)) {
    if (!isCoverageSourcePath(record.sourcePath)) {
      continue;
    }

    const existing = merged.get(record.sourcePath);
    if (existing === undefined) {
      merged.set(record.sourcePath, record);
    } else {
      mergeCountedRecord(existing, record);
    }
  }

  return [...merged.values()].map(serializeRecord).join("\n");
}

export function mergeLcovFiles({ basePath, supplementalPath, outputPath }) {
  const merged = mergeLcovReports(
    readFileSync(basePath, "utf8"),
    readFileSync(supplementalPath, "utf8"),
  );
  writeFileSync(outputPath, `${merged}\n`, "utf8");
}
