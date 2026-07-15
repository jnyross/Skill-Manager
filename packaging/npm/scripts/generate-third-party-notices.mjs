import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../..');
const goCache = fs.mkdtempSync(path.join(os.tmpdir(), 'skillet-notices-gocache-'));
const listed = spawnSync(
  'go',
  ['list', '-deps', '-f', '{{with .Module}}{{.Path}}\t{{.Version}}\t{{.Dir}}{{end}}', './cmd/skillet'],
  { cwd: repoRoot, encoding: 'utf8', env: { ...process.env, GOCACHE: goCache } },
);
if (listed.status !== 0) throw new Error(listed.stderr || listed.stdout);

const modules = new Map();
for (const line of listed.stdout.split('\n')) {
  if (!line.trim()) continue;
  const [modulePath, version, directory] = line.split('\t');
  if (!version || !directory || modulePath === 'github.com/jnyross/Skill-Manager') continue;
  modules.set(`${modulePath}@${version}`, { modulePath, version, directory });
}

const sections = [];
for (const module of [...modules.values()].sort((a, b) => a.modulePath.localeCompare(b.modulePath))) {
  const licenseFiles = fs.readdirSync(module.directory)
    .filter((name) => /^(LICENSE|LICENCE|COPYING|NOTICE)(\..*)?$/i.test(name))
    .sort();
  if (licenseFiles.length === 0) throw new Error(`no license file found for ${module.modulePath}@${module.version}`);
  const texts = licenseFiles.map((name) => {
    const text = fs.readFileSync(path.join(module.directory, name), 'utf8').trim();
    return `#### ${name}\n\n\`\`\`text\n${text}\n\`\`\``;
  });
  sections.push(`## ${module.modulePath} ${module.version}\n\n${texts.join('\n\n')}`);
}

const notice = `# Third-party notices\n\n` +
  `This file is generated from the exact Go dependency graph of \`./cmd/skillet\` by ` +
  `\`packaging/npm/scripts/generate-third-party-notices.mjs\`. Skillet is MIT-licensed; ` +
  `the following notices cover third-party modules included in the distributed binary.\n\n` +
  `${sections.join('\n\n')}\n`;
const rootNotice = path.join(repoRoot, 'THIRD_PARTY_NOTICES.md');
fs.writeFileSync(rootNotice, notice);
for (const directory of ['darwin-arm64', 'darwin-x64', 'linux-arm64', 'linux-x64']) {
  fs.copyFileSync(rootNotice, path.join(repoRoot, 'packaging/npm/packages', directory, 'THIRD_PARTY_NOTICES.md'));
}
console.log(`generated notices for ${modules.size} distributed Go modules`);
fs.rmSync(goCache, { recursive: true, force: true });
