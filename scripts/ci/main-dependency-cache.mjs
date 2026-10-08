import { execFileSync } from "node:child_process";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

export function warmMainDependencyCaches({ functionalHit, unitHit, execute = (args) => execFileSync("go", args, { encoding: "utf8" }) }) {
	if (functionalHit === "true" && unitHit === "true") return { warmed: false };
	const modulePath = execute(["list", "-m"]).trim();
	if (!modulePath) throw new Error("Missing root module identity.");
	const dependencies = execute(["list", "-deps", "-test", "-mod=readonly", "-f", "{{if not .ForTest}}{{.ImportPath}}{{end}}", "./pkg/...", "./tests/functional/..."]);
	const packages = [...new Set(dependencies.split(/\r?\n/).map((line) => line.trim()).filter((name) => name && name !== modulePath && !name.startsWith(`${modulePath}/`) && !name.endsWith(".test") && !name.includes("[")).concat("runtime/coverage"))].sort();
	execute(["build", "-p=4", ...packages]);
	return { warmed: true };
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
	warmMainDependencyCaches({ functionalHit: process.env.FUNCTIONAL_CACHE_HIT, unitHit: process.env.UNIT_CACHE_HIT });
}
