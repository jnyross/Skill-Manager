import crypto from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const npmRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const repoRoot = path.resolve(npmRoot, '../..');
const outputDir = path.resolve(process.argv[2] || path.join(npmRoot, 'artifacts'));
const packageDirectories = ['darwin-arm64', 'darwin-x64', 'linux-arm64', 'linux-x64', 'skillet'];
const npmCache = fs.mkdtempSync(path.join(os.tmpdir(), 'skillet-npm-pack-cache-'));

if (fs.existsSync(outputDir) && fs.readdirSync(outputDir).length !== 0) {
  throw new Error(`refusing to repack into non-empty directory: ${outputDir}`);
}
fs.mkdirSync(outputDir, { recursive: true });

const artifacts = [];
for (const directory of packageDirectories) {
  const packageDir = path.join(npmRoot, 'packages', directory);
  const manifest = JSON.parse(fs.readFileSync(path.join(packageDir, 'package.json'), 'utf8'));
  const result = spawnSync(
    'npm',
    ['pack', packageDir, '--pack-destination', outputDir, '--json', '--ignore-scripts'],
    { encoding: 'utf8', env: { ...process.env, npm_config_cache: npmCache } },
  );
  if (result.status !== 0) {
    throw new Error(`npm pack failed for ${manifest.name}:\n${result.stderr || result.stdout}`);
  }

  const packed = JSON.parse(result.stdout)[0];
  const filename = path.basename(packed.filename);
  const tarball = path.join(outputDir, filename);
  const bytes = fs.readFileSync(tarball);
  const expectedFiles = directory === 'skillet'
    ? ['LICENSE', 'README.md', 'bin/skillet.cjs', 'package.json']
    : ['LICENSE', 'README.md', 'THIRD_PARTY_NOTICES.md', 'bin/skillet', 'package.json'];
  const actualFiles = packed.files.map((entry) => entry.path).sort();
  assertEqual(actualFiles, expectedFiles.sort(), `${manifest.name} tarball files`);
  artifacts.push({
    name: manifest.name,
    version: manifest.version,
    filename,
    size: bytes.length,
    sha256: crypto.createHash('sha256').update(bytes).digest('hex'),
    shasum: packed.shasum,
    integrity: packed.integrity,
    files: actualFiles,
    manifest,
  });
}

const versions = new Set(artifacts.map((artifact) => artifact.version));
if (versions.size !== 1) throw new Error(`package versions disagree: ${[...versions].join(', ')}`);
const releaseDocuments = ['LICENSE', 'THIRD_PARTY_NOTICES.md'].map((filename) => {
  const source = path.join(repoRoot, filename);
  const destination = path.join(outputDir, filename);
  const bytes = fs.readFileSync(source);
  fs.writeFileSync(destination, bytes);
  return {
    filename,
    size: bytes.length,
    sha256: crypto.createHash('sha256').update(bytes).digest('hex'),
  };
});
const record = {
  schemaVersion: 1,
  version: artifacts[0].version,
  publicationOrder: artifacts.map((artifact) => artifact.name),
  artifacts,
  releaseDocuments,
};
fs.writeFileSync(path.join(outputDir, 'artifacts.json'), `${JSON.stringify(record, null, 2)}\n`);
const checksummedFiles = [
  ...artifacts.map((artifact) => ({ filename: artifact.filename, sha256: artifact.sha256 })),
  ...releaseDocuments,
];
fs.writeFileSync(
  path.join(outputDir, 'SHA256SUMS'),
  `${checksummedFiles.map((artifact) => `${artifact.sha256}  ${artifact.filename}`).join('\n')}\n`,
);
console.log(`packed and inspected ${artifacts.length} exact tarballs plus release license/notices in ${outputDir}`);
fs.rmSync(npmCache, { recursive: true, force: true });

function assertEqual(actual, expected, label) {
  if (JSON.stringify(actual) !== JSON.stringify(expected)) {
    throw new Error(`${label}: ${JSON.stringify(actual)} != ${JSON.stringify(expected)}`);
  }
}
