import { expect, test } from 'bun:test';
import { createHash } from 'node:crypto';
import { EventEmitter } from 'node:events';
import path from 'node:path';
import { installAndRun, parseArgs, runCommand } from './candidate-install-run.mjs';

const digest = (bytes) => createHash('sha256').update(bytes).digest('hex');
function fixture(assetId = 'windows-amd64') {
  const [os, cpu] = assetId.split('-');
  const archive = `you_0.0.8-snapshot_abc_${os}_${cpu}.${os === 'windows' ? 'zip' : 'tar.gz'}`;
  const manifest = 'you_0.0.8-snapshot_abc_checksums.txt';
  const bytes = Buffer.from('controlled native binary');
  const archiveBytes = Buffer.from('controlled archive');
  const calls = [];
  const storage = new Map();
  const root = path.resolve('controlled-temp', 'you-candidate-unique');
  const signals = new EventEmitter();
  let names = [archive, manifest];
  let checksum = `${digest(archiveBytes)}  ${archive}\n`;
  const files = {
    readdir: async () => names,
    readFile: async (name) => name.endsWith(manifest) ? checksum : name.endsWith(archive) ? archiveBytes : storage.get(name) ?? bytes,
    lstat: async () => ({ isFile: () => true, mode: 0o100755 }),
    mkdtemp: async (prefix) => { calls.push(['mkdtemp', prefix]); return root; },
    mkdir: async (name) => calls.push(['mkdir', name]),
    writeFile: async (name, data, options) => { calls.push(['write', name, options]); storage.set(name, data); },
    chmod: async (...args) => calls.push(['chmod', ...args]),
    rm: async (...args) => calls.push(['rm', ...args]),
  };
  const runs = [];
  const run = async (command, args, options) => {
    runs.push({ command, args, options });
    return command === 'tar' ? bytes : Buffer.from('# Config\nfactory.json\nworkTypes');
  };
  let tick = 0;
  const options = { dist: 'controlled-dist', binary: 'controlled-extracted', assetId, timeoutMs: 120000 };
  const deps = { files, run, signals, temp: path.resolve('controlled-temp'), platform: os === 'windows' ? 'win32' : os,
    arch: cpu === 'amd64' ? 'x64' : cpu, env: { HOME: 'ambient', USERPROFILE: 'ambient', PATH: 'tools' }, now: () => tick++ * 10 };
  return { archive, manifest, bytes, root, calls, runs, signals, files, storage, options, deps,
    names: (value) => { names = value; }, checksum: (value) => { checksum = value; } };
}

test('argument defaults and literal paths', () => {
  expect(parseArgs(['--dist', 'a b', '--asset-id', 'linux-amd64', '--binary', 'x y'])).toEqual({ dist: 'a b', assetId: 'linux-amd64', binary: 'x y', timeoutMs: 120000 });
});
for (const tail of [['--timeout-ms', '0'], ['--timeout-ms', '1.5'], ['--timeout-ms', '2147483648'], ['--dist', 'duplicate'], ['--unknown', 'x'], ['--binary']]) {
  test(`rejects invalid arguments ${tail.join(' ')}`, () => {
    expect(() => parseArgs(['--dist', 'd', '--asset-id', 'linux-amd64', '--binary', 'b', ...tail])).toThrow();
  });
}

for (const assetId of ['linux-amd64', 'linux-arm64', 'darwin-amd64', 'darwin-arm64', 'windows-amd64']) {
  test(`installs bound bytes/mode and runs exact private command for ${assetId}`, async () => {
    const f = fixture(assetId);
    const result = await installAndRun(f.options, f.deps);
    expect(result).toMatchObject({ assetId, archive: f.archive, status: 'PASS', exitCode: 0, timings: { validateMs: 10, installMs: 10, runMs: 10 } });
    expect(f.runs).toHaveLength(2);
    const installed = f.runs[1];
    expect(installed.command).toBe(result.installedBinary);
    expect(installed.args).toEqual(['docs', 'config']);
    expect(installed.options.cwd).toBe(f.root);
    expect(installed.options.env.HOME).toBe(path.join(f.root, 'profile'));
    expect(installed.options.env.USERPROFILE).toBe(installed.options.env.HOME);
    expect(installed.options.env.PATH).toBe('tools');
    expect(f.storage.get(result.installedBinary)).toEqual(f.bytes);
    expect(f.calls).toContainEqual(['chmod', result.installedBinary, 0o755]);
    expect(f.calls.at(-1)).toEqual(['rm', f.root, { recursive: true, force: true }]);
    expect(f.signals.listenerCount('SIGTERM')).toBe(0);
  });
}

for (const failure of ['digest', 'duplicate archive', 'missing archive', 'duplicate checksum', 'malformed manifest', 'foreign runner', 'symlink', 'timeout']) {
  test(`validation ${failure} never installs or executes`, async () => {
    const f = fixture();
    if (failure === 'digest') f.checksum(`${'0'.repeat(64)}  ${f.archive}`);
    if (failure === 'duplicate archive') f.names([f.archive, f.archive.replace('abc', 'def'), f.manifest]);
    if (failure === 'missing archive') f.names([f.manifest]);
    if (failure === 'duplicate checksum') f.checksum(`${digest(Buffer.from('controlled archive'))}  ${f.archive}\n`.repeat(2));
    if (failure === 'malformed manifest') f.checksum('bad checksum');
    if (failure === 'foreign runner') f.deps.platform = 'linux';
    if (failure === 'symlink') f.files.lstat = async () => ({ isFile: () => false });
    if (failure === 'timeout') f.options.timeoutMs = 0;
    await expect(installAndRun(f.options, f.deps)).rejects.toThrow('phase=validate');
    expect(f.runs).toEqual([]);
    expect(f.calls).toEqual([]);
  });
}

for (const failure of ['extracted mismatch', 'archive command', 'install write', 'installed mismatch', 'child failure', 'bad docs', 'cancel', 'cleanup']) {
  test(`failure ${failure} refuses success and cleans only owned root`, async () => {
    const f = fixture();
    if (failure === 'extracted mismatch') f.deps.run = async () => Buffer.from('wrong binary');
    if (failure === 'archive command') f.deps.run = async () => { throw new Error('tar failure'); };
    if (failure === 'install write') f.files.writeFile = async () => { throw new Error('write failure'); };
    if (failure === 'installed mismatch') f.files.writeFile = async (name) => f.storage.set(name, Buffer.from('bad write'));
    if (failure === 'child failure' || failure === 'bad docs' || failure === 'cancel') {
      const original = f.deps.run;
      f.deps.run = async (...args) => {
        if (args[0] !== 'tar') {
          if (failure === 'child failure') throw new Error('exit=7');
          if (failure === 'bad docs') return Buffer.from('partial docs');
          f.signals.emit('SIGTERM');
        }
        return original(...args);
      };
    }
    if (failure === 'cleanup') f.files.rm = async (...args) => { f.calls.push(['rm', ...args]); throw new Error('cleanup refused'); };
    await expect(installAndRun(f.options, f.deps)).rejects.toThrow();
    expect(f.calls.filter((call) => call[0] === 'rm')).toEqual([['rm', f.root, { recursive: true, force: true }]]);
    expect(f.signals.listenerCount('SIGINT')).toBe(0);
  });
}

function commandFixture(finish) {
  const child = new EventEmitter();
  child.stdout = new EventEmitter();
  child.stderr = new EventEmitter();
  const kills = [];
  child.kill = (signal) => { kills.push(signal); return true; };
  const controller = new AbortController();
  let timer;
  let cleared = false;
  const options = { cwd: 'private', env: { HOME: 'private' }, signal: controller.signal, timeoutMs: 100, maxBytes: 1024 };
  const deps = { spawnChild: (command, args, config) => {
    expect(config).toEqual({ cwd: 'private', env: { HOME: 'private' }, shell: false, stdio: ['ignore', 'pipe', 'pipe'] });
    queueMicrotask(() => finish({ child, controller, fireTimer: () => timer() }));
    return child;
  }, setTimer: (callback) => { timer = callback; return 1; }, clearTimer: () => { cleared = true; } };
  return { child, controller, kills, options, deps, cleared: () => cleared };
}

test('command captures literal bytes and joins successful close', async () => {
  const f = commandFixture(({ child }) => { child.stdout.emit('data', Buffer.from('docs')); child.emit('close', 0, null); });
  expect(await runCommand('binary', ['docs', 'config'], f.options, f.deps)).toEqual(Buffer.from('docs'));
  expect(f.cleared()).toBe(true);
});
for (const failure of ['timeout', 'abort', 'spawn', 'signal', 'exit', 'output', 'kill refusal']) {
  test(`command ${failure} is nonzero and joins close`, async () => {
    let joined = false;
    const f = commandFixture(({ child, controller, fireTimer }) => {
      if (failure === 'kill refusal') child.kill = () => false;
      if (failure === 'timeout' || failure === 'kill refusal') fireTimer();
      if (failure === 'abort') controller.abort();
      if (failure === 'spawn') child.emit('error', new Error('ENOENT'));
      if (failure === 'output') child.stdout.emit('data', Buffer.alloc(1025));
      child.emit('close', failure === 'exit' ? 7 : 0, failure === 'signal' ? 'SIGTERM' : null);
      joined = true;
    });
    await expect(runCommand('binary', [], f.options, f.deps)).rejects.toThrow();
    expect(joined).toBe(true);
    expect(f.cleared()).toBe(true);
    if (failure === 'timeout' || failure === 'abort' || failure === 'output') expect(f.kills).toEqual(['SIGKILL']);
  });
}
