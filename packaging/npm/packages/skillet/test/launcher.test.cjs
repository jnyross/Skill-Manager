const assert = require('node:assert/strict');
const path = require('node:path');
const test = require('node:test');

const launcher = require('../bin/skillet.cjs');

const mappings = [
  ['darwin', 'arm64', '@jnyross/skillet-darwin-arm64'],
  ['darwin', 'x64', '@jnyross/skillet-darwin-x64'],
  ['linux', 'arm64', '@jnyross/skillet-linux-arm64'],
  ['linux', 'x64', '@jnyross/skillet-linux-x64'],
];

test('selects the exact package for every supported pair', () => {
  for (const [platform, arch, expected] of mappings) {
    assert.equal(launcher.selectPlatformPackage(platform, arch), expected);
  }
});

test('rejects unsupported targets and lists every supported pair', () => {
  assert.throws(
    () => launcher.selectPlatformPackage('win32', 'x64'),
    (error) => {
      assert.match(error.message, /unsupported platform win32\/x64/);
      for (const [platform, arch] of mappings) {
        assert.match(error.message, new RegExp(`${platform}/${arch}`));
      }
      return true;
    },
  );
});

test('missing optional package gives exact recovery command', () => {
  const stderr = [];
  const code = launcher.runLauncher({
    platform: 'darwin',
    arch: 'arm64',
    version: '0.1.0',
    argv: [],
    resolvePackage: () => {
      const error = new Error('not found');
      error.code = 'MODULE_NOT_FOUND';
      throw error;
    },
    stderr: (line) => stderr.push(line),
  });

  assert.equal(code, 1);
  assert.match(stderr.join('\n'), /@jnyross\/skillet-darwin-arm64@0\.1\.0/);
  assert.match(stderr.join('\n'), /npm install --global @jnyross\/skillet@0\.1\.0 --include=optional/);
});

test('refuses a native package whose version differs', () => {
  const stderr = [];
  const code = launcher.runLauncher({
    platform: 'linux',
    arch: 'x64',
    version: '0.1.0',
    argv: [],
    resolvePackage: () => '/packages/linux-x64/package.json',
    readPackage: () => ({ version: '0.1.1' }),
    assertExecutable: () => {},
    spawn: () => assert.fail('must not execute mismatched native package'),
    stderr: (line) => stderr.push(line),
  });

  assert.equal(code, 1);
  assert.match(stderr.join('\n'), /version mismatch/);
  assert.match(stderr.join('\n'), /expected 0\.1\.0, found 0\.1\.1/);
});

test('executes without a shell and propagates args and exit status', () => {
  let call;
  const code = launcher.runLauncher({
    platform: 'linux',
    arch: 'arm64',
    version: '0.1.0',
    argv: ['--version'],
    resolvePackage: () => '/packages/linux-arm64/package.json',
    readPackage: () => ({ version: '0.1.0' }),
    assertExecutable: (binary) => assert.equal(binary, path.join('/packages/linux-arm64', 'bin', 'skillet')),
    spawn: (binary, args, options) => {
      call = { binary, args, options };
      return { status: 7, signal: null };
    },
    stderr: () => {},
  });

  assert.equal(code, 7);
  assert.deepEqual(call.args, ['--version']);
  assert.equal(call.options.shell, false);
  assert.equal(call.options.stdio, 'inherit');
});

test('propagates a child terminating signal', () => {
  const signals = [];
  const code = launcher.runLauncher({
    platform: 'darwin',
    arch: 'x64',
    version: '0.1.0',
    argv: [],
    resolvePackage: () => '/packages/darwin-x64/package.json',
    readPackage: () => ({ version: '0.1.0' }),
    assertExecutable: () => {},
    spawn: () => ({ status: null, signal: 'SIGTERM' }),
    signalSelf: (signal) => signals.push(signal),
    stderr: () => {},
  });

  assert.equal(code, 1);
  assert.deepEqual(signals, ['SIGTERM']);
});
