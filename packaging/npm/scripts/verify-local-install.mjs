import assert from 'node:assert/strict';
import crypto from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawn, spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { startLocalRegistry } from './local-registry.mjs';

const npmRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const artifactDirectory = path.resolve(process.argv[2] || path.join(npmRoot, 'artifacts'));
const record = JSON.parse(fs.readFileSync(path.join(artifactDirectory, 'artifacts.json'), 'utf8'));
const versionParts = record.version.split('.');
const brokenTestVersion = `${versionParts[0]}.${versionParts[1]}.${Number(versionParts[2]) + 1}-broken-test`;
const expectedPlatformPackage = platformPackage(process.platform, process.arch);
if (process.env.EXPECTED_PLATFORM_PAIR) {
  assert.equal(`${process.platform}/${process.arch}`, process.env.EXPECTED_PLATFORM_PAIR);
}
const tempRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'skillet-npm-gate-'));
const home = path.join(tempRoot, 'home');
const prefix = path.join(tempRoot, 'prefix');
const omittedPrefix = path.join(tempRoot, 'omitted-prefix');
const normalPrefix = path.join(tempRoot, 'normal-prefix');
fs.mkdirSync(home, { recursive: true });
const priorVersion = '0.0.9';
const priorArtifacts = createPriorArtifacts(priorVersion, expectedPlatformPackage);

const launcherManifest = record.artifacts.find((artifact) => artifact.name === '@jnyross/skillet')?.manifest;
assert(launcherManifest, 'launcher manifest missing from artifact record');
const registry = await startLocalRegistry({
  artifactDirectory,
  extraArtifacts: priorArtifacts,
  brokenVersions: {
    '@jnyross/skillet': {
      [brokenTestVersion]: { ...launcherManifest, optionalDependencies: mapValues(launcherManifest.optionalDependencies, () => brokenTestVersion) },
    },
  },
});
try {
  await npmInstall(prefix, ['@jnyross/skillet@' + priorVersion], registry.origin);
  let installed = path.join(prefix, 'bin', 'skillet');
  const priorIdentity = run(installed, ['--version'], { HOME: home });
  assert.equal(priorIdentity.status, 0, priorIdentity.stderr);
  assert.match(priorIdentity.stdout, new RegExp(`^skillet ${escapeRegex(priorVersion)} `));

  const stateFiles = {
    '.skillet/library/sentinel.json': '{"library":true}\n',
    '.skillet/bundles/sentinel.json': '{"bundle":true}\n',
    '.skillet/archive/sentinel/SKILL.md': '# archived\n',
    '.skillet/suppressions/sentinel.json': '{"suppressed":true}\n',
    'unrelated-user-state.txt': 'do not touch\n',
  };
  for (const [relative, contents] of Object.entries(stateFiles)) {
    const filename = path.join(home, relative);
    fs.mkdirSync(path.dirname(filename), { recursive: true });
    fs.writeFileSync(filename, contents);
  }

  await npmInstall(prefix, ['@jnyross/skillet@' + record.version], registry.origin);
  installed = path.join(prefix, 'bin', 'skillet');
  const version = run(installed, ['--version'], { HOME: home });
  assert.equal(version.status, 0, version.stderr);
  assert.match(version.stdout, new RegExp(`^skillet ${escapeRegex(record.version)} \\(commit .+, built .+\\)\\n$`));

  const installedScope = path.join(prefix, 'lib', 'node_modules', '@jnyross');
  const installedPackages = findPackageNames(installedScope);
  assert(installedPackages.includes('skillet'), `launcher missing: ${installedPackages}`);
  assert(installedPackages.includes(expectedPlatformPackage.split('/')[1]), `native package missing: ${installedPackages}`);
  for (const candidate of ['skillet-darwin-arm64', 'skillet-darwin-x64', 'skillet-linux-arm64', 'skillet-linux-x64']) {
    assert.equal(installedPackages.includes(candidate), candidate === expectedPlatformPackage.split('/')[1]);
  }

  smokeTUI(installed, home);

  await npmInstallNormal(normalPrefix, ['@jnyross/skillet@' + record.version], registry.origin);
  const normalInstalled = path.join(normalPrefix, 'bin', 'skillet');
  const normalVersion = run(normalInstalled, ['--version'], { HOME: home });
  assert.equal(normalVersion.status, 0, normalVersion.stderr);
  assert.equal(normalVersion.stdout, version.stdout, 'normal install and --ignore-scripts install resolved different command identities');
  smokeTUI(normalInstalled, home);
  for (const [relative, contents] of Object.entries(stateFiles)) {
    assert.equal(fs.readFileSync(path.join(home, relative), 'utf8'), contents, `${relative} changed during upgrade`);
  }

  const failedUpgrade = await npmInstallResult(prefix, ['@jnyross/skillet@' + brokenTestVersion], registry.origin);
  assert.notEqual(failedUpgrade.status, 0, 'broken upgrade unexpectedly succeeded');
  const afterFailure = run(installed, ['--version'], { HOME: home });
  assert.equal(afterFailure.status, 0, afterFailure.stderr);
  assert.match(afterFailure.stdout, new RegExp(`^skillet ${escapeRegex(record.version)} `));
  smokeTUI(installed, home);

  const incompleteRegistry = await startLocalRegistry({
    artifactDirectory,
    includePackages: ['@jnyross/skillet'],
  });
  try {
    await npmInstall(
      omittedPrefix,
      ['--omit', 'optional', '@jnyross/skillet@' + record.version],
      incompleteRegistry.origin,
    );
    const omitted = run(path.join(omittedPrefix, 'bin', 'skillet'), ['--version'], { HOME: home });
    assert.notEqual(omitted.status, 0);
    assert.match(omitted.stderr, new RegExp(`missing optional native package.*${escapeRegex(expectedPlatformPackage)}@${escapeRegex(record.version)}`, 's'));
    assert.match(omitted.stderr, new RegExp(`npm install --global @jnyross/skillet@${escapeRegex(record.version)} --include=optional`));
  } finally {
    await incompleteRegistry.close();
  }

  const platformOnlyRegistry = await startLocalRegistry({
    artifactDirectory,
    includePackages: record.artifacts.filter((artifact) => artifact.name !== '@jnyross/skillet').map((artifact) => artifact.name),
  });
  try {
    const launcherView = await spawnResult('npm', ['view', `@jnyross/skillet@${record.version}`, '--registry', platformOnlyRegistry.origin]);
    assert.notEqual(launcherView.status, 0, 'launcher became resolvable before launcher-last publication');
    const nativeView = await spawnResult('npm', ['view', `${expectedPlatformPackage}@${record.version}`, 'version', '--registry', platformOnlyRegistry.origin]);
    assert.equal(nativeView.status, 0, nativeView.stderr);
    assert.equal(nativeView.stdout.trim(), record.version);
  } finally {
    await platformOnlyRegistry.close();
  }

  console.log(`verified local-registry install and ${priorVersion} -> ${record.version} upgrade on ${process.platform}/${process.arch}, including failure preservation`);
} finally {
  await registry.close();
  if (process.env.SKILLET_KEEP_TEMP === '1') {
    console.error(`preserved verification directory: ${tempRoot}`);
  } else {
    fs.rmSync(tempRoot, { recursive: true, force: true });
  }
}

async function npmInstall(targetPrefix, packages, registryOrigin) {
  const result = await npmInstallResult(targetPrefix, packages, registryOrigin, true);
  assert.equal(result.status, 0, result.stderr || result.stdout);
}

async function npmInstallNormal(targetPrefix, packages, registryOrigin) {
  const result = await npmInstallResult(targetPrefix, packages, registryOrigin, false);
  assert.equal(result.status, 0, result.stderr || result.stdout);
}

function npmInstallResult(targetPrefix, packages, registryOrigin, ignoreScripts = true) {
  return spawnResult('npm', [
    'install', '--global',
    '--prefix', targetPrefix,
    '--cache', path.join(tempRoot, 'npm-cache'),
    '--registry', registryOrigin,
    ...(ignoreScripts ? ['--ignore-scripts'] : []),
    '--no-audit', '--no-fund',
    ...packages,
  ]);
}

function spawnResult(command, args) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, { cwd: tempRoot, stdio: ['ignore', 'pipe', 'pipe'] });
    let stdout = '';
    let stderr = '';
    child.stdout.setEncoding('utf8');
    child.stderr.setEncoding('utf8');
    child.stdout.on('data', (chunk) => { stdout += chunk; });
    child.stderr.on('data', (chunk) => { stderr += chunk; });
    child.once('error', reject);
    child.once('close', (status, signal) => resolve({ status, signal, stdout, stderr }));
  });
}

function run(command, args, env = {}) {
  return spawnSync(command, args, { encoding: 'utf8', env: { ...process.env, ...env } });
}

function smokeTUI(command, testHome) {
  const env = { ...process.env, HOME: testHome, TERM: process.env.TERM || 'xterm-256color' };
  const result = spawnSync(
    'python3',
    [path.join(npmRoot, 'scripts', 'pty-smoke.py'), command],
    { encoding: 'utf8', env },
  );
  assert.equal(result.status, 0, result.stderr || result.stdout);
}

function findPackageNames(scopeDirectory) {
  if (!fs.existsSync(scopeDirectory)) return [];
  const direct = fs.readdirSync(scopeDirectory);
  const nestedScope = path.join(scopeDirectory, 'skillet', 'node_modules', '@jnyross');
  return [...new Set([...direct, ...(fs.existsSync(nestedScope) ? fs.readdirSync(nestedScope) : [])])];
}

function platformPackage(platform, arch) {
  const packageName = {
    'darwin/arm64': '@jnyross/skillet-darwin-arm64',
    'darwin/x64': '@jnyross/skillet-darwin-x64',
    'linux/arm64': '@jnyross/skillet-linux-arm64',
    'linux/x64': '@jnyross/skillet-linux-x64',
  }[`${platform}/${arch}`];
  if (!packageName) throw new Error(`unsupported verification host ${platform}/${arch}`);
  return packageName;
}

function escapeRegex(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function createPriorArtifacts(version, nativePackageName) {
  const priorRoot = path.join(tempRoot, 'prior');
  const output = path.join(priorRoot, 'artifacts');
  const nativeDirectory = nativePackageName.split('/')[1].replace('skillet-', '');
  const packageDirectories = ['skillet', nativeDirectory];
  fs.mkdirSync(output, { recursive: true });

  for (const directory of packageDirectories) {
    const source = path.join(npmRoot, 'packages', directory);
    const destination = path.join(priorRoot, 'packages', directory);
    fs.cpSync(source, destination, { recursive: true });
    const manifestFile = path.join(destination, 'package.json');
    const manifest = JSON.parse(fs.readFileSync(manifestFile, 'utf8'));
    manifest.version = version;
    if (manifest.optionalDependencies) {
      manifest.optionalDependencies = mapValues(manifest.optionalDependencies, () => version);
    }
    fs.writeFileSync(manifestFile, `${JSON.stringify(manifest, null, 2)}\n`);
  }

  const nativeBinary = path.join(priorRoot, 'packages', nativeDirectory, 'bin', 'skillet');
  fs.mkdirSync(path.dirname(nativeBinary), { recursive: true });
  const built = spawnSync('go', [
    'build', '-trimpath',
    '-ldflags', `-s -w -X main.version=${version} -X main.commit=prior-fixture -X main.buildDate=2026-07-15T00:00:00Z`,
    '-o', nativeBinary, './cmd/skillet',
  ], {
    cwd: path.resolve(npmRoot, '../..'),
    encoding: 'utf8',
    env: {
      ...process.env,
      CGO_ENABLED: '0',
      GOCACHE: path.join(tempRoot, 'prior-go-cache'),
      GOOS: process.platform === 'darwin' ? 'darwin' : 'linux',
      GOARCH: process.arch === 'x64' ? 'amd64' : 'arm64',
    },
  });
  assert.equal(built.status, 0, built.stderr || built.stdout);
  fs.chmodSync(nativeBinary, 0o755);

  return packageDirectories.map((directory) => {
    const packageDirectory = path.join(priorRoot, 'packages', directory);
    const manifest = JSON.parse(fs.readFileSync(path.join(packageDirectory, 'package.json'), 'utf8'));
    const packed = spawnSync('npm', [
      'pack', packageDirectory, '--pack-destination', output, '--json', '--ignore-scripts',
    ], {
      cwd: tempRoot,
      encoding: 'utf8',
      env: { ...process.env, npm_config_cache: path.join(tempRoot, 'prior-npm-cache') },
    });
    assert.equal(packed.status, 0, packed.stderr || packed.stdout);
    const details = JSON.parse(packed.stdout)[0];
    const filename = path.basename(details.filename);
    const tarballPath = path.join(output, filename);
    const bytes = fs.readFileSync(tarballPath);
    return {
      name: manifest.name,
      version,
      filename,
      path: tarballPath,
      shasum: crypto.createHash('sha1').update(bytes).digest('hex'),
      integrity: `sha512-${crypto.createHash('sha512').update(bytes).digest('base64')}`,
      manifest,
    };
  });
}

function mapValues(value, mapper) {
  return Object.fromEntries(Object.entries(value).map(([key, item]) => [key, mapper(item, key)]));
}
