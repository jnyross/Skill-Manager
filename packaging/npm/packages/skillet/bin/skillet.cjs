#!/usr/bin/env node
'use strict';

const fs = require('node:fs');
const path = require('node:path');
const { spawnSync } = require('node:child_process');

const launcherPackage = require('../package.json');

const platformPackages = new Map([
  ['darwin/arm64', '@jnyross/skillet-darwin-arm64'],
  ['darwin/x64', '@jnyross/skillet-darwin-x64'],
  ['linux/arm64', '@jnyross/skillet-linux-arm64'],
  ['linux/x64', '@jnyross/skillet-linux-x64'],
]);

function selectPlatformPackage(platform, arch) {
  const pair = `${platform}/${arch}`;
  const packageName = platformPackages.get(pair);
  if (packageName) return packageName;

  throw new Error(
    `unsupported platform ${pair}; supported platforms: ${[...platformPackages.keys()].join(', ')}`,
  );
}

function runLauncher(options = {}) {
  const platform = options.platform ?? process.platform;
  const arch = options.arch ?? process.arch;
  const version = options.version ?? launcherPackage.version;
  const argv = options.argv ?? process.argv.slice(2);
  const resolvePackage = options.resolvePackage ?? ((name) => require.resolve(`${name}/package.json`, { paths: [__dirname] }));
  const readPackage = options.readPackage ?? ((filename) => JSON.parse(fs.readFileSync(filename, 'utf8')));
  const assertExecutable = options.assertExecutable ?? ((filename) => fs.accessSync(filename, fs.constants.X_OK));
  const spawn = options.spawn ?? spawnSync;
  const signalSelf = options.signalSelf ?? ((signal) => process.kill(process.pid, signal));
  const stderr = options.stderr ?? ((line) => console.error(line));

  let packageName;
  try {
    packageName = selectPlatformPackage(platform, arch);
  } catch (error) {
    stderr(`skillet: ${error.message}`);
    return 1;
  }

  let packageJSONPath;
  try {
    packageJSONPath = resolvePackage(packageName);
  } catch (error) {
    stderr(
      `skillet: missing optional native package for ${platform}/${arch}: ${packageName}@${version}\n` +
      `reinstall with: npm install --global @jnyross/skillet@${version} --include=optional`,
    );
    return 1;
  }

  try {
    const nativePackage = readPackage(packageJSONPath);
    if (nativePackage.version !== version) {
      throw new Error(`native package version mismatch: expected ${version}, found ${nativePackage.version}`);
    }

    const binary = path.join(path.dirname(packageJSONPath), 'bin', 'skillet');
    assertExecutable(binary);
    const result = spawn(binary, argv, { stdio: 'inherit', shell: false });
    if (result.error) throw result.error;
    if (result.signal) {
      signalSelf(result.signal);
      return 1;
    }
    if (!Number.isInteger(result.status)) {
      throw new Error('native process exited without a status or signal');
    }
    return result.status;
  } catch (error) {
    stderr(`skillet: ${error.message}`);
    return 1;
  }
}

if (require.main === module) {
  process.exitCode = runLauncher();
}

module.exports = { runLauncher, selectPlatformPackage };
