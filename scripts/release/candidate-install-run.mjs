import { spawn } from 'node:child_process';
import { createHash } from 'node:crypto';
import * as fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';

const assets = {
  'linux-amd64': ['linux', 'x64', 'linux_amd64.tar.gz', 'you'],
  'linux-arm64': ['linux', 'arm64', 'linux_arm64.tar.gz', 'you'],
  'darwin-amd64': ['darwin', 'x64', 'darwin_amd64.tar.gz', 'you'],
  'darwin-arm64': ['darwin', 'arm64', 'darwin_arm64.tar.gz', 'you'],
  'windows-amd64': ['win32', 'x64', 'windows_amd64.zip', 'you.exe'],
};
const sha256 = (bytes) => createHash('sha256').update(bytes).digest('hex');

export function parseArgs(args) {
  const options = { timeoutMs: 120000 };
  const names = { '--dist': 'dist', '--asset-id': 'assetId', '--binary': 'binary', '--timeout-ms': 'timeoutMs' };
  const seen = new Set();
  for (let i = 0; i < args.length; i += 2) {
    const key = names[args[i]];
    if (!key || seen.has(key) || !args[i + 1] || args[i + 1].startsWith('--')) throw new Error('invalid candidate arguments');
    seen.add(key);
    options[key] = args[i + 1];
  }
  if (!options.dist || !options.binary || !assets[options.assetId]) throw new Error('dist, binary and supported asset-id required');
  if (!/^\d+$/.test(String(options.timeoutMs))) throw new Error('timeout-ms must be a positive integer');
  options.timeoutMs = Number(options.timeoutMs);
  if (!Number.isSafeInteger(options.timeoutMs) || options.timeoutMs <= 0 || options.timeoutMs > 2147483647) throw new Error('timeout-ms out of range');
  return options;
}

// Timeout/cancellation terminate the direct command and join close before cleanup.
// The only commands used here (tar and docs config) do not launch workers/servers.
export async function runCommand(command, args, options, {
  spawnChild = spawn, setTimer = setTimeout, clearTimer = clearTimeout,
} = {}) {
  options.signal.throwIfAborted();
  const child = spawnChild(command, args, { cwd: options.cwd, env: options.env, shell: false, stdio: ['ignore', 'pipe', 'pipe'] });
  const chunks = [];
  const errors = [];
  let size = 0;
  let failure;
  const stop = (error) => {
    failure ||= error;
    try { if (!child.kill('SIGKILL')) failure = new Error(`${failure.message}; termination refused`); }
    catch (error) { failure = new Error(`${failure.message}; ${error.message}`); }
  };
  const abort = () => stop(new Error('command cancelled'));
  child.stdout.on('data', (chunk) => {
    size += chunk.length;
    if (size > options.maxBytes) stop(new Error('command output limit exceeded'));
    else chunks.push(Buffer.from(chunk));
  });
  let errorSize = 0;
  child.stderr.on('data', (chunk) => { errorSize += chunk.length; if (errorSize <= 65536) errors.push(Buffer.from(chunk)); });
  const closed = new Promise((resolve) => {
    child.on('error', (error) => { failure ||= error; });
    child.once('close', (exitCode, signal) => resolve({ exitCode, signal }));
  });
  options.signal.addEventListener('abort', abort, { once: true });
  const timer = setTimer(() => stop(new Error(`command timed out after ${options.timeoutMs}ms`)), options.timeoutMs);
  if (options.signal.aborted) abort();
  try {
    const result = await closed;
    if (failure) throw failure;
    if (result.exitCode !== 0 || result.signal) throw new Error(`command exit=${result.exitCode} signal=${result.signal}: ${Buffer.concat(errors).toString()}`);
    return Buffer.concat(chunks);
  } finally {
    clearTimer(timer);
    options.signal.removeEventListener('abort', abort);
  }
}

export async function installAndRun(options, {
  files = fs, run = runCommand, platform = process.platform, arch = process.arch,
  temp = os.tmpdir(), env = process.env, signals = process, now = () => performance.now(),
} = {}) {
  const asset = assets[options.assetId];
  let phase = 'validate';
  let ownedRoot;
  const timings = {};
  const started = now();
  const controller = new AbortController();
  const cancel = () => controller.abort();
  signals.on('SIGINT', cancel);
  signals.on('SIGTERM', cancel);
  try {
    if (!asset || platform !== asset[0] || arch !== asset[1]) throw new Error('asset does not match native runner');
    if (!Number.isInteger(options.timeoutMs) || options.timeoutMs <= 0 || options.timeoutMs > 2147483647) throw new Error('invalid timeout');
    const dist = path.resolve(options.dist);
    const names = await files.readdir(dist);
    const archives = names.filter((name) => name.startsWith('you_') && name.endsWith(`_${asset[2]}`));
    const manifests = names.filter((name) => name.startsWith('you_') && name.endsWith('_checksums.txt'));
    if (archives.length !== 1 || manifests.length !== 1) throw new Error('require exactly one native archive and checksum manifest');
    const archive = archives[0];
    const manifest = await files.readFile(path.join(dist, manifests[0]), 'utf8');
    const entries = manifest.split(/\r?\n/).filter(Boolean).map((line) => /^([a-fA-F0-9]{64})\s+\*?([^/\\]+)$/.exec(line));
    if (entries.some((entry) => !entry)) throw new Error('malformed checksum manifest');
    const matches = entries.filter((entry) => entry[2] === archive);
    if (matches.length !== 1) throw new Error('require exactly one archive checksum');
    const digest = sha256(await files.readFile(path.join(dist, archive)));
    if (digest !== matches[0][1].toLowerCase()) throw new Error('archive checksum mismatch');
    const source = path.resolve(options.binary);
    const stat = await files.lstat(source);
    if (!stat.isFile()) throw new Error('extracted binary must be a regular file');
    controller.signal.throwIfAborted();
    timings.validateMs = now() - started;
    phase = 'install';
    ownedRoot = await files.mkdtemp(path.join(temp, 'you-candidate-'));
    const bin = path.join(ownedRoot, 'bin');
    const profile = path.join(ownedRoot, 'profile');
    await files.mkdir(bin);
    await files.mkdir(profile);
    const isolatedEnv = { ...env, HOME: profile, USERPROFILE: profile, HOMEDRIVE: path.parse(profile).root.replace(/[\\/]$/, ''), HOMEPATH: path.sep,
      XDG_CONFIG_HOME: path.join(profile, '.config'), XDG_CACHE_HOME: path.join(profile, '.cache'), APPDATA: profile, LOCALAPPDATA: profile };
    const commandOptions = { cwd: ownedRoot, env: isolatedEnv, signal: controller.signal, timeoutMs: options.timeoutMs };
    // Read only the named member, never unpack arbitrary archive paths. Bind the
    // caller's extracted bytes to the verified archive before installing them.
    const member = await run('tar', ['-xOf', path.join(dist, archive), asset[3]], { ...commandOptions, maxBytes: 512 * 1024 * 1024 });
    if (!member.length || sha256(member) !== sha256(await files.readFile(source))) throw new Error('extracted binary differs from verified archive');
    const installedBinary = path.join(bin, asset[3]);
    await files.writeFile(installedBinary, member, { mode: stat.mode & 0o777 });
    await files.chmod(installedBinary, stat.mode & 0o777);
    if (sha256(await files.readFile(installedBinary)) !== sha256(member)) throw new Error('installed binary mismatch');
    timings.installMs = now() - started - timings.validateMs;
    phase = 'run';
    const output = (await run(installedBinary, ['docs', 'config'], { ...commandOptions, maxBytes: 4 * 1024 * 1024 })).toString();
    if (!['# Config', 'factory.json', 'workTypes'].every((text) => output.includes(text))) throw new Error('installed docs config output missing required content');
    controller.signal.throwIfAborted();
    timings.runMs = now() - started - timings.validateMs - timings.installMs;
    return { assetId: options.assetId, archive, sha256: digest, installedBinary, command: [installedBinary, 'docs', 'config'], exitCode: 0, status: 'PASS', timings };
  } catch (error) {
    throw new Error(`${options.assetId} phase=${phase}: ${error.message}`, { cause: error });
  } finally {
    signals.removeListener('SIGINT', cancel);
    signals.removeListener('SIGTERM', cancel);
    // Only the exact mkdtemp result is owned; caller dist/binary are untouched.
    if (ownedRoot) {
      const cleanupStarted = now();
      try { await files.rm(ownedRoot, { recursive: true, force: true }); }
      catch (error) { throw new Error(`${options.assetId} phase=cleanup: ${error.message}`, { cause: error }); }
      timings.cleanupMs = now() - cleanupStarted;
    }
  }
}

if (import.meta.main) {
  try { process.stdout.write(`${JSON.stringify(await installAndRun(parseArgs(process.argv.slice(2))))}\n`); }
  catch (error) { process.stderr.write(`${JSON.stringify({ status: 'FAIL', error: error.message })}\n`); process.exitCode = 1; }
}
