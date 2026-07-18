import { execFileSync } from "node:child_process";
import { mkdirSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";

const [, , inputPathArg, outputPathArg] = process.argv;

if (!inputPathArg || !outputPathArg) {
  console.error(
    "Usage: node scripts/generate-openapi-types.mjs <input-openapi-path> <output-ts-path>",
  );
  process.exit(1);
}

const projectRoot = resolve(import.meta.dirname, "..");
const inputPath = resolve(projectRoot, inputPathArg);
const outputPath = resolve(projectRoot, outputPathArg);

const generatedSource = execFileSync(
  process.execPath,
  ["./node_modules/openapi-typescript/bin/cli.js", inputPath, "--enum"],
  {
    cwd: projectRoot,
    encoding: "utf8",
  },
);

const transformedSource = generatedSource.replace(
  /^export enum (\w+) \{\n([\s\S]*?)^\}/gm,
  (_match, enumName, enumBody) => {
    const transformedBody = enumBody.replace(
      /^(\s*)([A-Za-z0-9_]+)\s*=\s*(.+?)(,?)$/gm,
      (_memberMatch, indentation, memberName, memberValue, trailingComma) =>
        `${indentation}${memberName}: ${memberValue}${trailingComma || ","}`,
    );

    return [
      `export const ${enumName} = {`,
      transformedBody,
      `} as const;`,
      `export type ${enumName} = (typeof ${enumName})[keyof typeof ${enumName}];`,
    ].join("\n");
  },
);

mkdirSync(dirname(outputPath), { recursive: true });
writeFileSync(outputPath, transformedSource);

execFileSync(
  process.execPath,
  ["./node_modules/@biomejs/biome/bin/biome", "format", "--write", outputPath],
  {
    cwd: projectRoot,
    stdio: "inherit",
  },
);
