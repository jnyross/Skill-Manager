import fs from 'node:fs';
import http from 'node:http';
import path from 'node:path';

export async function startLocalRegistry({ artifactDirectory, extraArtifacts = [], brokenVersions = {}, includePackages }) {
  const record = JSON.parse(fs.readFileSync(path.join(artifactDirectory, 'artifacts.json'), 'utf8'));
  const artifacts = [...record.artifacts, ...extraArtifacts].filter(
    (artifact) => !includePackages || includePackages.includes(artifact.name),
  );
  const packages = new Map();
  for (const artifact of artifacts) {
    const versions = packages.get(artifact.name) || new Map();
    versions.set(artifact.version, artifact);
    packages.set(artifact.name, versions);
  }

  let origin;
  const server = http.createServer((request, response) => {
    const requestURL = new URL(request.url, origin || 'http://127.0.0.1');
    const pathname = decodeURIComponent(requestURL.pathname);
    if (pathname === '/-/ping') return sendJSON(response, 200, {});

    if (pathname.startsWith('/tarballs/')) {
      const filename = path.basename(pathname);
      const tarball = artifacts.find((artifact) => artifact.filename === filename);
      if (!tarball) return sendJSON(response, 404, { error: 'tarball not found' });
      response.writeHead(200, { 'content-type': 'application/octet-stream' });
      fs.createReadStream(tarball.path || path.join(artifactDirectory, filename)).pipe(response);
      return;
    }

    const packageName = pathname.slice(1);
    const versions = packages.get(packageName);
    const broken = brokenVersions[packageName] || {};
    if (!versions && Object.keys(broken).length === 0) {
      return sendJSON(response, 404, { error: `package ${packageName} not found` });
    }

    const metadataVersions = {};
    for (const artifact of versions?.values() || []) {
      metadataVersions[artifact.version] = withDistribution(artifact.manifest, artifact, origin);
    }
    for (const [version, manifest] of Object.entries(broken)) {
      metadataVersions[version] = {
        ...manifest,
        name: packageName,
        version,
        dist: {
          tarball: `${origin}/tarballs/missing-${encodeURIComponent(packageName)}-${version}.tgz`,
          shasum: '0000000000000000000000000000000000000000',
        },
      };
    }
    const sortedVersions = Object.keys(metadataVersions).sort(compareVersions);
    sendJSON(response, 200, {
      name: packageName,
      'dist-tags': { latest: sortedVersions.at(-1) },
      versions: metadataVersions,
    });
  });

  await new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen(0, '127.0.0.1', resolve);
  });
  const address = server.address();
  origin = `http://127.0.0.1:${address.port}`;
  return {
    origin,
    close: () => new Promise((resolve, reject) => server.close((error) => error ? reject(error) : resolve())),
  };
}

function withDistribution(manifest, artifact, origin) {
  return {
    ...manifest,
    dist: {
      tarball: `${origin}/tarballs/${artifact.filename}`,
      shasum: artifact.shasum,
      integrity: artifact.integrity,
    },
  };
}

function sendJSON(response, status, value) {
  response.writeHead(status, { 'content-type': 'application/json' });
  response.end(JSON.stringify(value));
}

function compareVersions(left, right) {
  return left.localeCompare(right, 'en', { numeric: true });
}
