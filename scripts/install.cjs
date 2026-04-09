const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const https = require('node:https');
const { spawnSync } = require('node:child_process');

const pkgRoot = path.resolve(__dirname, '..');
const vendorDir = path.join(pkgRoot, 'bin');
const binaryName = process.platform === 'win32' ? 'work-bridge.exe' : 'work-bridge';

const repo =
  process.env.WORK_BRIDGE_GITHUB_REPO ||
  process.env.npm_package_repository_url ||
  process.env.npm_package_repository ||
  '';
const releaseTag = process.env.WORK_BRIDGE_VERSION || process.env.npm_package_version || 'latest';

if (!repo) {
  console.warn('[work-bridge] WORK_BRIDGE_GITHUB_REPO is not set; skipping binary download.');
  process.exit(0);
}

const [owner, name] = repo.replace(/^git\+https:\/\/github.com\//, '').replace(/^https:\/\/github.com\//, '').replace(/\.git$/, '').split('/').slice(-2);
if (!owner || !name) {
  console.warn('[work-bridge] Could not parse GitHub repository from WORK_BRIDGE_GITHUB_REPO; skipping binary download.');
  process.exit(0);
}

const platform = platformId(process.platform);
const arch = archId(process.arch);
if (!platform || !arch) {
  console.warn(`[work-bridge] Unsupported platform ${process.platform}/${process.arch}; skipping binary download.`);
  process.exit(0);
}

const assetName = `work-bridge_${platform}_${arch}.tar.gz`;
const url = `https://github.com/${owner}/${name}/releases/download/${releaseTag}/${assetName}`;
const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'work-bridge-'));
const archivePath = path.join(tmpDir, assetName);

download(url, archivePath, (err) => {
  if (err) {
    console.warn(`[work-bridge] Download failed: ${err.message}`);
    process.exit(0);
  }

  fs.mkdirSync(vendorDir, { recursive: true });
  const extract = spawnSync('tar', ['-xzf', archivePath, '-C', vendorDir], { stdio: 'inherit' });
  if (extract.status !== 0) {
    console.warn('[work-bridge] Extraction failed; the wrapper will remain inert until a successful reinstall.');
    process.exit(0);
  }

  const extractedBinary = path.join(vendorDir, binaryName);
  if (process.platform !== 'win32' && fs.existsSync(extractedBinary)) {
    fs.chmodSync(extractedBinary, 0o755);
  }
  process.exit(0);
});

function download(url, dest, done) {
  const file = fs.createWriteStream(dest);
  const request = https.get(url, (response) => {
    if (response.statusCode >= 300 && response.statusCode < 400 && response.headers.location) {
      file.close();
      return download(response.headers.location, dest, done);
    }
    if (response.statusCode !== 200) {
      file.close();
      return done(new Error(`unexpected status ${response.statusCode}`));
    }
    response.pipe(file);
    file.on('finish', () => {
      file.close(done);
    });
  });

  request.on('error', (err) => {
    file.close();
    done(err);
  });
}

function platformId(platform) {
  switch (platform) {
    case 'darwin':
    case 'linux':
      return platform;
    case 'win32':
      return 'windows';
    default:
      return '';
  }
}

function archId(arch) {
  switch (arch) {
    case 'x64':
      return 'amd64';
    case 'arm64':
      return 'arm64';
    default:
      return '';
  }
}
