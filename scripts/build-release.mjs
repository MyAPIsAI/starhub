import { spawnSync } from 'node:child_process';
import { mkdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { join, resolve } from 'node:path';
import { parseArgs } from 'node:util';

const root = fileURLToPath(new URL('../', import.meta.url));
const web = join(root, 'web');
const operatingSystems = ['windows', 'linux', 'darwin'];
const architectures = ['amd64', 'arm64'];
const help = `Build StarHub on Windows 11, Linux, or macOS.

Usage: node scripts/build-release.mjs [options]

Options:
  --os <windows|linux|darwin>  Target OS (darwin means macOS)
  --arch <amd64|arm64>        Target architecture (Intel/AMD or ARM/Apple Silicon)
  --all                     Build all six supported OS/architecture pairs
  --backend-only            Build without the web console or npm dependencies
  --skip-checks             Skip lint, formatting, type checks, vet, and tests
  --output <directory>      Output directory (default: <repository>/release)
  --help                    Show this help

Requirements: Node.js 24+ and Go 1.26+; npm is required for console builds.
Defaults: build the host platform, including the embedded web console.
GOOS/GOARCH environment variables may also select a target. CLI options win.
Checks always run on the host; target binaries are compiled with CGO_ENABLED=0.
--all ignores GOOS/GOARCH and cannot be combined with --os or --arch.
Relative --output paths are resolved from the current working directory.

Examples:
  node scripts/build-release.mjs
  node scripts/build-release.mjs --os linux --arch amd64
  node scripts/build-release.mjs --os darwin --arch arm64
  node scripts/build-release.mjs --all
  node scripts/build-release.mjs --backend-only --all
`;

function run(command, args, { cwd = root, env = process.env, capture = false } = {}) {
  const windowsNpm = command === 'npm' && process.platform === 'win32';
  // Only fixed npm commands enter cmd.exe. Paths and user options never enter a shell.
  const executable = windowsNpm ? (process.env.ComSpec ?? 'cmd.exe') : command;
  const parameters = windowsNpm ? ['/d', '/s', '/c', ['npm', ...args].join(' ')] : args;
  if (!capture) console.log(`\n> ${command} ${args.join(' ')}`);
  const result = spawnSync(executable, parameters, {
    cwd,
    stdio: capture ? ['ignore', 'pipe', 'pipe'] : 'inherit',
    encoding: 'utf8',
    env,
    windowsHide: true,
  });
  if (result.error?.code === 'ENOENT') {
    throw new Error(`${command} was not found. Install it and add it to PATH.`);
  }
  if (result.error) throw result.error;
  if (result.status !== 0) {
    const detail = capture ? result.stderr?.trim() : '';
    throw new Error(`${command} failed (${result.signal ?? result.status}).${detail ? `\n${detail}` : ''}`);
  }
  return result.stdout?.trim() ?? '';
}

function targetEnvironment(os, arch) {
  return { ...process.env, GOOS: os, GOARCH: arch, CGO_ENABLED: '0' };
}

function main() {
  const { values } = parseArgs({
    options: {
      os: { type: 'string' },
      arch: { type: 'string' },
      all: { type: 'boolean', default: false },
      'backend-only': { type: 'boolean', default: false },
      'skip-checks': { type: 'boolean', default: false },
      output: { type: 'string' },
      help: { type: 'boolean', default: false },
    },
    allowPositionals: false,
  });
  if (values.help) { console.log(help); return; }
  if (Number(process.versions.node.split('.')[0]) < 24) {
    throw new Error('Node.js 24 or newer is required.');
  }
  if (values.all && (values.os !== undefined || values.arch !== undefined)) {
    throw new Error('--all cannot be combined with --os or --arch.');
  }
  if (values.output !== undefined && !values.output.trim()) {
    throw new Error('--output must name a directory.');
  }

  // Query the actual Go host, independently of cross-compilation settings.
  const go = JSON.parse(run('go', ['env', '-json', 'GOHOSTOS', 'GOHOSTARCH', 'GOVERSION'], { capture: true }));
  const version = /go(\d+)\.(\d+)/.exec(go.GOVERSION);
  if (!version || Number(version[1]) < 1 || (Number(version[1]) === 1 && Number(version[2]) < 26)) {
    throw new Error(`Go 1.26 or newer is required; found ${go.GOVERSION}.`);
  }
  const targets = values.all
    ? operatingSystems.flatMap(os => architectures.map(arch => ({ os, arch })))
    : [{ os: values.os ?? (process.env.GOOS || go.GOHOSTOS), arch: values.arch ?? (process.env.GOARCH || go.GOHOSTARCH) }];
  for (const { os, arch } of targets) {
    if (!operatingSystems.includes(os) || !architectures.includes(arch)) {
      throw new Error(`Unsupported target ${os}/${arch}. Use windows, linux, or darwin with amd64 or arm64.`);
    }
  }

  const hostEnv = targetEnvironment(go.GOHOSTOS, go.GOHOSTARCH);
  const withConsole = !values['backend-only'];
  const checks = !values['skip-checks'];
  const output = values.output === undefined ? join(root, 'release') : resolve(values.output);
  const namedTarget = values.all || values.os !== undefined || values.arch !== undefined || Boolean(process.env.GOOS || process.env.GOARCH);
  const tagArgs = withConsole ? ['-tags', 'frontend'] : [];

  console.log(`Host: ${go.GOHOSTOS}/${go.GOHOSTARCH} (${go.GOVERSION}, Node ${process.versions.node})`);
  console.log(`Targets: ${targets.map(({ os, arch }) => `${os}/${arch}`).join(', ')}`);
  console.log(`Console: ${withConsole ? 'embedded' : 'excluded'}; checks: ${checks ? 'enabled' : 'skipped'}`);
  console.log(`Output: ${output}`);

  if (withConsole) {
    run('npm', ['--version'], { cwd: web, env: hostEnv, capture: true });
    run('npm', ['ci'], { cwd: web, env: hostEnv });
    if (checks) {
      for (const check of ['lint', 'format:check', 'typecheck', 'test:unit']) {
        run('npm', ['run', check], { cwd: web, env: hostEnv });
      }
    }
    run('npm', ['run', 'build'], { cwd: web, env: hostEnv });
  }
  if (checks) {
    // Never try to execute foreign-platform test binaries during a cross build.
    for (const check of ['build', 'vet', 'test']) {
      run('go', [check, ...tagArgs, './...'], { env: hostEnv });
    }
  }
  mkdirSync(output, { recursive: true });
  const artifacts = [];
  for (const { os, arch } of targets) {
    const name = `starhub${withConsole ? '' : '-backend'}${namedTarget ? `-${os}-${arch}` : ''}${os === 'windows' ? '.exe' : ''}`;
    const destination = join(output, name);
    run('go', ['build', '-trimpath', ...tagArgs, '-o', destination, '.'], { env: targetEnvironment(os, arch) });
    artifacts.push(destination);
  }
  console.log(`\nBuild completed:\n${artifacts.map(path => `  ${path}`).join('\n')}`);
  console.log('\nRun the generated binary above; an older binary in the repository root is not updated.');
  const nativeIndex = targets.findIndex(({ os, arch }) => os === go.GOHOSTOS && arch === go.GOHOSTARCH);
  if (nativeIndex !== -1) {
    const binary = artifacts[nativeIndex];
    const launch = process.platform === 'win32'
      ? `& '${binary.replaceAll("'", "''")}'`
      : `'${binary.replaceAll("'", "'\\''")}'`;
    console.log(`${process.platform === 'win32' ? 'PowerShell' : 'Shell'}: ${launch}`);
    console.log(withConsole
      ? 'Web console: open http://localhost:3000/ (or your STARHUB_PORT). Stop the previous server before starting this binary.'
      : 'API-only build: no web console is included; / returns 404.');
  }
}

try {
  main();
} catch (error) {
  console.error(`\nBuild failed: ${error.message}`);
  process.exitCode = 1;
}
