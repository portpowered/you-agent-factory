import { BUN_UNIT_TEST_GLOB } from "./ui-test-lane-contract.mjs";

const bunUnitTestFiles = [
  ...new Bun.Glob(BUN_UNIT_TEST_GLOB).scanSync({
    cwd: process.cwd(),
  }),
].sort();
const bunTestArguments = process.argv.slice(2);

if (bunUnitTestFiles.length === 0) {
  console.error(
    `[ui-unit:bun] no files matched ${BUN_UNIT_TEST_GLOB}; the Bun unit lane must not pass without an owned test`,
  );
  process.exitCode = 1;
} else {
  console.log(
    `[ui-unit:bun] discovered ${bunUnitTestFiles.length} file(s): ${bunUnitTestFiles.join(", ")}`,
  );
  console.log(
    `[ui-unit:bun] expected seed test count: 1; Bun will report the terminal file and test totals below`,
  );

  const result = Bun.spawnSync({
    cmd: [process.execPath, "test", ...bunTestArguments, ...bunUnitTestFiles],
    cwd: process.cwd(),
    stderr: "inherit",
    stdout: "inherit",
  });

  process.exitCode = result.exitCode;
}
