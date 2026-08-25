import { spawnSync } from 'node:child_process';
import { mkdirSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const scriptDir = dirname(fileURLToPath(import.meta.url));
const backendDir = resolve(scriptDir, '..', 'backend');
const distDir = resolve(scriptDir, '..', 'src-tauri', 'binaries');
mkdirSync(distDir, { recursive: true });

const targets = [
  { goos: 'windows', goarch: 'amd64', triple: 'x86_64-pc-windows-msvc', ext: '.exe' },
  { goos: 'darwin', goarch: 'amd64', triple: 'x86_64-apple-darwin', ext: '' },
  { goos: 'darwin', goarch: 'arm64', triple: 'aarch64-apple-darwin', ext: '' },
  { goos: 'linux', goarch: 'amd64', triple: 'x86_64-unknown-linux-gnu', ext: '' },
  { goos: 'linux', goarch: 'arm64', triple: 'aarch64-unknown-linux-gnu', ext: '' },
];

const requestedTriple = process.argv.slice(2).find((arg) =>
  targets.some((target) => target.triple === arg),
) || '';

const unknownArgs = process.argv.slice(2).filter((arg) => arg !== requestedTriple);
if (unknownArgs.length > 0) {
  console.error(`unsupported argument: ${unknownArgs[0]}`);
  process.exit(1);
}

const selectedTargets = requestedTriple
  ? targets.filter((target) => target.triple === requestedTriple)
  : targets;

if (selectedTargets.length === 0) {
  console.error(`unsupported target: ${requestedTriple}`);
  process.exit(1);
}

for (const target of selectedTargets) {
  const name = `mkrouter-core-${target.triple}${target.ext}`;
  const out = join(distDir, name);
  console.log(`building ${name}`);
  const result = spawnSync(
    'go',
    ['build', '-trimpath', '-ldflags', '-s -w', '-o', out, '.'],
    {
      cwd: backendDir,
      stdio: 'inherit',
      env: {
        ...process.env,
        CGO_ENABLED: '0',
        GOOS: target.goos,
        GOARCH: target.goarch,
      },
    },
  );
  if (result.status !== 0) {
    process.exit(result.status ?? 1);
  }
}

console.log(`done -> ${distDir}`);
