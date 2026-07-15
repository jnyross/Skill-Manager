import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const npmRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const workspace = readJSON(path.join(npmRoot, 'package.json'));
const expectedVersion = process.env.RELEASE_VERSION || workspace.version;
const requireBinaries = process.argv.includes('--require-binaries');
const repositoryURL = 'git+https://github.com/jnyross/Skill-Manager.git';

const platformSpecs = {
  'darwin-arm64': { name: '@jnyross/skillet-darwin-arm64', os: 'darwin', cpu: 'arm64' },
  'darwin-x64': { name: '@jnyross/skillet-darwin-x64', os: 'darwin', cpu: 'x64' },
  'linux-arm64': { name: '@jnyross/skillet-linux-arm64', os: 'linux', cpu: 'arm64' },
  'linux-x64': { name: '@jnyross/skillet-linux-x64', os: 'linux', cpu: 'x64' },
};

assert(workspace.private === true, 'workspace root must remain private');
assert(workspace.version === expectedVersion, `workspace version ${workspace.version} != ${expectedVersion}`);

const launcherDir = path.join(npmRoot, 'packages', 'skillet');
const launcher = readJSON(path.join(launcherDir, 'package.json'));
validateCommon(launcher, '@jnyross/skillet', 'skillet');
assert(launcher.bin?.skillet === 'bin/skillet.cjs', 'launcher bin must be bin/skillet.cjs');
assert(launcher.engines?.node === '>=22.14.0', 'consumer Node floor drifted');
assert(launcher.engines?.npm === '>=10.9.0', 'consumer npm floor drifted');
assertNoLifecycleScripts(launcher);
assertSet(launcher.files, ['LICENSE', 'README.md', 'bin/skillet.cjs'], 'launcher files');
const launcherSource = fs.readFileSync(path.join(launcherDir, 'bin', 'skillet.cjs'), 'utf8');
assert(launcherSource.startsWith('#!/usr/bin/env node\n'), 'launcher must start with npm-compatible Node shebang');

for (const [directory, spec] of Object.entries(platformSpecs)) {
  const packageDir = path.join(npmRoot, 'packages', directory);
  const manifest = readJSON(path.join(packageDir, 'package.json'));
  validateCommon(manifest, spec.name, directory);
  assertSet(manifest.os, [spec.os], `${spec.name} os`);
  assertSet(manifest.cpu, [spec.cpu], `${spec.name} cpu`);
  assert(!('libc' in manifest), `${spec.name} must not declare libc`);
  assertNoLifecycleScripts(manifest);
  assertSet(manifest.files, ['LICENSE', 'README.md', 'THIRD_PARTY_NOTICES.md', 'bin/skillet'], `${spec.name} files`);
  assert(launcher.optionalDependencies?.[spec.name] === expectedVersion, `${spec.name} optional dependency must equal ${expectedVersion}`);
  if (requireBinaries) {
    fs.accessSync(path.join(packageDir, 'bin', 'skillet'), fs.constants.X_OK);
  }
}

const expectedOptionalNames = Object.values(platformSpecs).map((spec) => spec.name);
assertSet(Object.keys(launcher.optionalDependencies || {}), expectedOptionalNames, 'launcher optional dependency names');
console.log(`validated five Skillet npm packages at ${expectedVersion}`);

function validateCommon(manifest, expectedName, directory) {
  assert(manifest.name === expectedName, `${directory} name ${manifest.name} != ${expectedName}`);
  assert(manifest.version === expectedVersion, `${expectedName} version ${manifest.version} != ${expectedVersion}`);
  assert(manifest.license === 'MIT', `${expectedName} license must be MIT`);
  assert(manifest.repository?.type === 'git', `${expectedName} repository type must be git`);
  assert(manifest.repository?.url === repositoryURL, `${expectedName} repository URL case or value drifted`);
  assert(manifest.repository?.directory === `packaging/npm/packages/${directory}`, `${expectedName} repository directory drifted`);
  assert(manifest.publishConfig?.access === 'public', `${expectedName} must publish as public`);
  for (const filename of manifest.files || []) {
    if (filename === 'bin/skillet' && !requireBinaries) continue;
    fs.accessSync(path.join(npmRoot, 'packages', directory, filename), fs.constants.R_OK);
  }
}

function assertNoLifecycleScripts(manifest) {
  for (const name of ['preinstall', 'install', 'postinstall']) {
    assert(!manifest.scripts?.[name], `${manifest.name} must not define ${name}`);
  }
}

function assertSet(actual, expected, label) {
  assert(Array.isArray(actual), `${label} must be an array`);
  const a = [...actual].sort();
  const e = [...expected].sort();
  assert(JSON.stringify(a) === JSON.stringify(e), `${label} ${JSON.stringify(a)} != ${JSON.stringify(e)}`);
}

function readJSON(filename) {
  return JSON.parse(fs.readFileSync(filename, 'utf8'));
}

function assert(condition, message) {
  if (!condition) throw new Error(message);
}
