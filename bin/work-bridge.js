#!/usr/bin/env node

const { spawnSync } = require('node:child_process');
const { existsSync } = require('node:fs');
const { join } = require('node:path');

const binary = join(__dirname, 'work-bridge');

if (!existsSync(binary)) {
  console.error('work-bridge binary is not installed yet. Re-run npm install or check the release assets.');
  process.exit(1);
}

const result = spawnSync(binary, process.argv.slice(2), { stdio: 'inherit' });
process.exit(result.status ?? 1);
