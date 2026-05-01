#!/usr/bin/env node

const path = require('node:path');
const { spawnSync } = require('node:child_process');

const SCRIPT_DIR = __dirname;


function nodeExecutable() {
  return process.execPath;
}

function scriptPath(filename) {
  return path.join(SCRIPT_DIR, filename);
}

function npmExecutable(platform = process.platform) {
  return platform === 'win32' ? 'npm.cmd' : 'npm';
}

function redoclyExecutable(cwd = process.cwd(), platform = process.platform) {
  const executable = platform === 'win32' ? 'redocly.cmd' : 'redocly';
  return path.join(cwd, 'node_modules', '.bin', executable);
}

function specRootArg(commandArgs = []) {
  return commandArgs[0] || 'openapi-main.yaml';
}

function specOutputArg(commandArgs = []) {
  return commandArgs[1] || 'openapi.yaml';
}

function redoclyCommand(platform = process.platform) {
  return npmExecutable(platform);
}

function redoclyArgs(commandArgs, platform = process.platform) {
  const redoclyBinary = platform === 'win32' ? 'redocly.cmd' : 'redocly';
  return ['exec', '--package', '@redocly/cli', '--', redoclyBinary, ...commandArgs];
}

function commandLine(command, args) {
  return [command, ...args].join(' ');
}

function needsShell(command, platform = process.platform) {
  return platform === 'win32' && /\.cmd$/i.test(command);
}

function phaseDefinitions(cwd = process.cwd(), platform = process.platform, commandArgs = []) {
  const nodeCommand = nodeExecutable();
  const specRoot = specRootArg(commandArgs);
  const specOutput = specOutputArg(commandArgs);

  return {
    'bundle:rest': {
      label: 'OpenAPI REST bundle',
      command: redoclyCommand(platform),
      args: redoclyArgs(['bundle', specRoot, '--output', specOutput], platform),
    },
    validate: {
      label: 'OpenAPI validation',
      command: redoclyCommand(platform),
      args: redoclyArgs(['lint', 'openapi.yaml'], platform),
    },
    'validate:main': {
      label: 'OpenAPI main validation',
      command: redoclyCommand(platform),
      args: redoclyArgs(['lint', specRoot], platform),
    }
  };
}

function commandDefinitions(cwd = __dirname, platform = process.platform, commandArgs = []) {
  const phases = phaseDefinitions(cwd, platform, commandArgs);

  return {
    ...phases,
    bundle: {
      label: 'OpenAPI bundle',
      phases: [
        phases['bundle:rest']
      ],
    },
  };
}

function successLine(commandName) {
  switch (commandName) {
    case 'bundle':
      return '[api:bundle] OpenAPI, flow-node configs, and capability interfaces bundled.';
    case 'bundle:rest':
      return '[api:bundle] OpenAPI REST bundle generated.';
   
    case 'validate':
      return '[api:validate] OpenAPI bundle validated.';
    case 'validate:main':
      return '[api:validate] OpenAPI main spec validated.';
    default:
      return `[api] ${commandName} passed.`;
  }
}

function failureCode(result) {
  if (typeof result.status === 'number') {
    return `exit code ${result.status}`;
  }

  if (result.signal) {
    return `signal ${result.signal}`;
  }

  return 'unknown failure';
}

function runPhase(phase, options) {
  const verbose = options.verbose;
  const spawn = options.spawn ?? spawnSync;
  const platform = options.platform ?? process.platform;
  const stdout = options.stdout ?? process.stdout;
  const stderr = options.stderr ?? process.stderr;

  const result = spawn(phase.command, phase.args, {
    cwd: options.cwd,
    encoding: 'utf8',
    env: options.env,
    shell: needsShell(phase.command, platform),
    stdio: verbose ? 'inherit' : ['ignore', 'pipe', 'pipe'],
  });

  if (result.error) {
    stderr.write(`[api] FAILED ${phase.label}. Command: ${commandLine(phase.command, phase.args)}\n`);
    stderr.write(`[api] ${result.error.message}\n`);
    return 1;
  }

  const exitCode = result.status ?? 1;
  if (exitCode === 0) {
    return 0;
  }

  stderr.write(`[api] FAILED ${phase.label}. Command: ${commandLine(phase.command, phase.args)}\n`);
  if (result.stdout) {
    stdout.write(result.stdout);
  }
  if (result.stderr) {
    stderr.write(result.stderr);
  }
  stderr.write(`[api] ${failureCode(result)}\n`);

  return exitCode;
}

function runApiCommand(commandName, options = {}) {
  const cwd = options.cwd ?? process.cwd();
  const env = options.env ?? process.env;
  const platform = options.platform ?? process.platform;
  const stdout = options.stdout ?? process.stdout;
  const commandArgs = options.commandArgs ?? [];
  const definitions = options.definitions ?? commandDefinitions(cwd, platform, commandArgs);
  const definition = definitions[commandName];

  if (!definition) {
    throw new Error(`Unknown API command: ${commandName}`);
  }

  const verbose = false;
  const phases = definition.phases ?? [definition];

  for (const phase of phases) {
    const exitCode = runPhase(phase, {
      ...options,
      cwd,
      env,
      platform,
      verbose,
    });

    if (exitCode !== 0) {
      return exitCode;
    }
  }

  if (!verbose) {
    stdout.write(`${successLine(commandName)}\n`);
  }

  return 0;
}

function main(argv = process.argv.slice(2)) {
  const commandName = argv[0];
  if (!commandName) {
    console.error('Usage: node run-quiet-api-command.js <bundle|bundle:rest|validate|validate:main> [spec-root] [spec-output]');
    process.exitCode = 1;
    return;
  }

  try {
    process.exitCode = runApiCommand(commandName, { commandArgs: argv.slice(1) });
  } catch (error) {
    console.error(`[api] ${error.message}`);
    process.exitCode = 1;
  }
}

if (require.main === module) {
  main();
}

module.exports = {
  commandDefinitions,
  commandLine,
  needsShell,
  npmExecutable,
  redoclyArgs,
  redoclyCommand,
  phaseDefinitions,
  redoclyExecutable,
  runApiCommand,
  runPhase,
  SCRIPT_DIR,
  scriptPath,
  specOutputArg,
  specRootArg,
  successLine,
};
