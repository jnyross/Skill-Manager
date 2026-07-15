import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const sourceRoot = path.resolve(process.env.SKILLET_CATALOG_SOURCE_ROOT || '/private/tmp/skillet-catalog-sources');
const reviewedDate = '2026-07-15';

const sources = {
  matt: source('matt', 'https://github.com/mattpocock/skills.git', 'e9fcdf95b402d360f90f1db8d776d5dd450f9234', 'MIT', 'LICENSE', 'license-text'),
  superpowers: source('superpowers', 'https://github.com/obra/superpowers.git', 'd884ae04edebef577e82ff7c4e143debd0bbec99', 'MIT', 'LICENSE', 'license-text'),
  vercel: source('vercel', 'https://github.com/vercel-labs/agent-skills.git', 'f8a72b9603728bb92a217a879b7e62e43ad76c81', 'MIT', 'README.md', 'declaration-only'),
  anthropic: source('anthropic', 'https://github.com/anthropics/skills.git', '9d2f1ae187231d8199c64b5b762e1bdf2244733d', 'Apache-2.0', null, 'license-text'),
  dotnet: source('dotnet', 'https://github.com/dotnet/skills.git', '79a2ada302fe8feb884ab33e1bfc2bddae3ee7ce', 'MIT', 'LICENSE', 'license-text'),
};

const specs = [
  member('matt', 'walking-skeleton', 'ask-matt', 'skills/engineering/ask-matt'),
  ...members('matt', 'matt-engineering', [
    'engineering/code-review', 'engineering/codebase-design',
    'engineering/diagnosing-bugs', 'engineering/domain-modeling', 'engineering/grill-with-docs',
    'engineering/implement', 'engineering/improve-codebase-architecture', 'engineering/prototype',
    'engineering/research', 'engineering/setup-matt-pocock-skills', 'engineering/tdd',
    'engineering/to-spec', 'engineering/to-tickets', 'engineering/triage', 'engineering/wayfinder',
  ], 'skills/'),
  ...members('matt', 'matt-collaboration', [
    'productivity/grill-me', 'productivity/grilling', 'productivity/handoff',
    'productivity/teach', 'productivity/writing-great-skills',
  ], 'skills/'),
  ...members('superpowers', 'superpowers-workflow', [
    'brainstorming', 'dispatching-parallel-agents', 'executing-plans', 'finishing-a-development-branch',
    'receiving-code-review', 'requesting-code-review', 'subagent-driven-development',
    'systematic-debugging', 'test-driven-development', 'using-git-worktrees', 'using-superpowers',
    'verification-before-completion', 'writing-plans', 'writing-skills',
  ], 'skills/'),
  member('vercel', 'vercel-frontend', 'web-design-guidelines', 'skills/web-design-guidelines'),
  member('vercel', 'vercel-frontend', 'vercel-react-best-practices', 'skills/react-best-practices'),
  member('vercel', 'vercel-frontend', 'vercel-composition-patterns', 'skills/composition-patterns'),
  member('vercel', 'vercel-frontend', 'writing-guidelines', 'skills/writing-guidelines'),
  ...['skill-creator', 'frontend-design', 'webapp-testing'].map((name) => member('anthropic', 'anthropic-ui', name, `skills/${name}`)),
  member('dotnet', 'dotnet-starter', 'setup-local-sdk', 'plugins/dotnet/skills/setup-local-sdk'),
  member('dotnet', 'dotnet-starter', 'dotnet-webapi', 'plugins/dotnet-aspnetcore/skills/dotnet-webapi'),
  member('dotnet', 'dotnet-starter', 'optimizing-ef-core-queries', 'plugins/dotnet-data/skills/optimizing-ef-core-queries'),
  member('dotnet', 'dotnet-starter', 'analyzing-dotnet-performance', 'plugins/dotnet-diag/skills/analyzing-dotnet-performance'),
  member('dotnet', 'dotnet-starter', 'run-tests', 'plugins/dotnet-test/skills/run-tests'),
  member('dotnet', 'dotnet-starter', 'writing-mstest-tests', 'plugins/dotnet-test/skills/writing-mstest-tests'),
];

const catalog = {
  schemaVersion: 1,
  version: '2026.07.15.2',
  reviewedDate,
  members: specs.map(buildMember),
  bundles: [
    bundle('walking-skeleton', 'Walking skeleton', specs),
    bundle('matt-engineering', 'Matt Pocock engineering', specs),
    bundle('matt-collaboration', 'Matt Pocock collaboration', specs),
    { ...bundle('superpowers-workflow', 'Superpowers workflow', specs), overlapWarning: 'Overlaps planning, TDD, debugging, review, and implementation workflows in the Matt family', explicitSelectionOnly: true },
    bundle('vercel-frontend', 'Vercel frontend guidance', specs),
    bundle('anthropic-ui', 'Anthropic creator and UI', specs),
    { ...bundle('dotnet-starter', '.NET starter', specs), recommendationGlobs: ['*.sln', '*.slnx', '*.csproj', 'global.json'], explicitSelectionOnly: true },
  ],
};

fs.writeFileSync(
  path.join(repoRoot, 'internal/catalog/catalog.json'),
  `${JSON.stringify(catalog, null, 2)}\n`,
);
console.log(`generated catalog ${catalog.version} with ${catalog.members.length} exact members`);

function source(directory, repository, revision, spdx, notice, evidence) {
  return { directory, repository, revision, spdx, notice, evidence };
}

function member(family, bundleID, name, subpath) {
  return { family, bundleID, name, subpath };
}

function members(family, bundleID, paths, prefix) {
  return paths.map((entry) => member(family, bundleID, path.basename(entry), `${prefix}${entry}`));
}

function buildMember(spec) {
  const sourceSpec = sources[spec.family];
  const directory = path.join(sourceRoot, sourceSpec.directory, spec.subpath);
  if (!fs.statSync(directory).isDirectory()) throw new Error(`missing catalog boundary ${directory}`);
  const skillFile = path.join(directory, 'SKILL.md');
  const skillSource = fs.readFileSync(skillFile, 'utf8');
  const declaredName = frontmatterValue(skillSource, 'name');
  if (declaredName && declaredName !== spec.name) {
    throw new Error(`${spec.subpath} declares ${declaredName}, expected ${spec.name}`);
  }
  const noticeFile = sourceSpec.notice
    ? path.join(sourceRoot, sourceSpec.directory, sourceSpec.notice)
    : path.join(directory, 'LICENSE.txt');
  const notice = fs.readFileSync(noticeFile);
  const scripts = walkFiles(directory)
    .filter((filename) => /(^|\/)(scripts?|bin)\/|\.(py|sh|js|mjs|cjs|ts|rb|ps1)$/i.test(filename));
	const executables = scripts.filter((filename) => (fs.statSync(path.join(directory, filename)).mode & 0o111) !== 0);
	const boundaryText = walkFiles(directory)
		.filter((filename) => /\.(md|txt|ya?ml|json|py|sh|js|mjs|cjs|ts|rb|ps1)$/i.test(filename))
		.map((filename) => fs.readFileSync(path.join(directory, filename), 'utf8'))
		.join('\n');
  return {
    name: spec.name,
    family: spec.family,
    source: {
      repository: sourceSpec.repository,
      subpath: spec.subpath,
      reviewedRevision: sourceSpec.revision,
      reviewedDate,
      contentSHA256: hashTree(directory),
			metadataSHA256: operationalEvidenceHash(directory, ['name:', 'description:', 'disable-model-invocation:', 'allowed-tools:', 'user-invocable:']),
			dependencyEvidenceSHA256: operationalEvidenceHash(directory, ['git ', 'gh ', 'npm ', 'npx ', 'python', 'playwright', 'dotnet', 'curl', 'wget', 'download', 'network', 'browser']),
			externalActionEvidenceSHA256: operationalEvidenceHash(directory, ['issue create', 'publish', 'git commit', 'git push', 'create a branch', 'write', 'edit', 'create', 'save', 'generate', 'download', 'install', 'browser', 'screenshot']),
    },
    license: {
      spdx: sourceSpec.spdx,
      notice: path.relative(path.join(sourceRoot, sourceSpec.directory), noticeFile),
      noticeSHA256: sha256(notice),
      evidence: sourceSpec.evidence,
    },
    upstreamActivation: /^disable-model-invocation:\s*true\s*$/mi.test(skillSource) ? 'manual-only' : 'auto',
    dependencies: dependencies(spec, boundaryText, scripts),
    scripts,
		executables,
    externalActions: externalActions(spec, boundaryText),
    verificationPrompt: `Use $${spec.name} only to identify yourself. Do not run tools, scripts, commands, downloads, network calls, or external actions. Return only SKILLET_DISCOVERED_${spec.name.replaceAll('-', '_').toUpperCase()}.`,
    recipes: [
      { tool: 'claude-code', scope: 'project', artifact: 'direct-skill', requires: [] },
      { tool: 'codex', scope: 'project', artifact: 'direct-skill', requires: [] },
    ],
  };
}

function bundle(id, name, allSpecs) {
  return { id, name, members: allSpecs.filter((spec) => spec.bundleID === id).map((spec) => spec.name) };
}


function dependencies(spec, text, scripts) {
  const result = [];
  const add = (name, reason) => {
    if (!result.some((dependency) => dependency.name === name)) result.push({ name, optional: true, reason });
  };
  if (spec.family === 'dotnet') {
    add('dotnet', 'Operational workflow requires a compatible .NET SDK or diagnostic toolchain');
    if (spec.name === 'setup-local-sdk') {
      add('network', 'The skill may download a project-local SDK only when explicitly invoked');
    }
  }
  if (spec.name === 'webapp-testing') {
    add('playwright', 'Operational browser testing requires Playwright; discovery does not');
  }
  if (/\b(?:git (?:commit|diff|log|status|worktree|branch|merge|push)|merge-base)\b/i.test(text)) {
    add('git', 'Operational workflow uses Git; guided setup and discovery do not invoke it');
  }
  if (/\bgh (?:issue|api|pr|repo)\b/i.test(text)) {
    add('gh', 'Operational workflow uses the authenticated GitHub CLI; guided setup and discovery do not invoke it');
    add('network', 'Operational GitHub actions require network access; guided setup and discovery do not');
  }
	if (['setup-matt-pocock-skills', 'to-spec', 'to-tickets', 'triage', 'wayfinder'].includes(spec.name)) {
		add('gh', 'Operational tracker workflow requires an authenticated GitHub surface; guided setup and discovery do not invoke it');
		add('network', 'Operational tracker workflow requires network access; guided setup and discovery do not');
	}
  if (/\b(?:npm|npx)\b/i.test(text)) add('npm', 'Operational workflow may use Node package tooling; discovery does not');
  if (/\b(?:python3|python |pytest|\.py\b)/i.test(text) || scripts.some((name) => name.endsWith('.py'))) add('python3', 'Operational workflow includes Python commands or scripts; discovery does not');
  if (/\b(?:curl|wget|download|fetch from|web search|browser)\b/i.test(text)) add('network', 'Operational workflow may require network access; guided setup and discovery do not');
  return result.sort((left, right) => left.name.localeCompare(right.name));
}

function externalActions(spec, text) {
  const actions = [];
  const add = (value) => { if (!actions.includes(value)) actions.push(value); };
  if (/\bgh issue (?:create|edit|close|comment)\b|\bpublish(?:es|ing)?\b.*\bissue/i.test(text)) {
    add('May create or mutate GitHub issues when explicitly invoked and authorized; guided setup never performs tracker mutations');
  }
	if (['setup-matt-pocock-skills', 'to-spec', 'to-tickets', 'triage', 'wayfinder'].includes(spec.name)) {
		add('May create or mutate GitHub issues, labels, or dependencies when explicitly invoked and authorized; guided setup never performs tracker mutations');
	}
  if (/\b(?:git commit|commit the|commits? changes|create a branch|merge the branch|git push)\b/i.test(text)) {
    add('May create Git commits, branches, merges, or pushes when explicitly invoked and authorized; guided setup never mutates Git history');
  }
  if (/\b(?:write|edit|create|save|generate)\b.{0,40}\b(?:file|document|spec|plan|report|artifact|code|test)/i.test(text)) {
    add('May create or edit project files when explicitly invoked and authorized; guided setup only writes its displayed managed destinations');
  }
  if (/\b(?:download|install)\b/i.test(text)) {
    add('May download or install operational dependencies when explicitly invoked and authorized; guided setup never runs member installers');
  }
  if (/\b(?:playwright|browser|screenshot)\b/i.test(text)) {
    add('May launch or control a browser and capture application output when explicitly invoked; guided setup never launches member tools');
  }
  if (spec.name === 'setup-local-sdk') add('May download and install a project-local .NET SDK when explicitly invoked; guided setup never invokes it');
  if (spec.name === 'webapp-testing') add('May launch a browser and exercise an application when explicitly invoked; guided setup never invokes it');
  return actions.sort();
}

function walkFiles(root) {
  const result = [];
  for (const entry of fs.readdirSync(root, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name))) {
    const absolute = path.join(root, entry.name);
    if (entry.isDirectory()) {
      for (const child of walkFiles(absolute)) result.push(path.join(entry.name, child).split(path.sep).join('/'));
    } else if (entry.isFile()) {
      result.push(entry.name);
    }
  }
  return result;
}

function hashTree(root) {
  const hash = crypto.createHash('sha256');
  for (const filename of walkFiles(root).sort()) {
    hash.update(filename);
    hash.update('\0');
    hash.update(fs.readFileSync(path.join(root, filename)));
    hash.update('\0');
  }
  return hash.digest('hex');
}

function operationalEvidenceHash(root, tokens) {
	const evidence = [];
	for (const filename of walkFiles(root).filter(isOperationalText).sort()) {
		const lines = fs.readFileSync(path.join(root, filename), 'utf8').toLowerCase().split('\n');
		lines.forEach((line, index) => {
			const trimmed = line.trim();
			if (tokens.some((token) => trimmed.includes(token))) evidence.push(`${filename}:${index + 1}:${trimmed}`);
		});
	}
	return sha256(evidence.sort().join('\n'));
}

function isOperationalText(filename) {
	const extension = path.extname(filename).toLowerCase();
	return extension === '' || ['.md', '.txt', '.yaml', '.yml', '.json', '.py', '.sh', '.js', '.mjs', '.cjs', '.ts', '.rb', '.ps1'].includes(extension);
}

function frontmatterValue(source, key) {
  const match = source.match(new RegExp(`^${key}:\\s*["']?([^\\n"']+)["']?\\s*$`, 'mi'));
  return match?.[1]?.trim();
}

function sha256(value) {
  return crypto.createHash('sha256').update(value).digest('hex');
}
