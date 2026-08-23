#!/usr/bin/env node
// Launches the Go backend in demo mode against a brand-new SQLite database,
// for Playwright's webServer to manage. A plain shell command can't
// reliably guarantee a fresh db file cross-platform (Windows cmd.exe has no
// `rm`, and stale state across runs would make write-flow tests flaky), so
// this wrapper creates a unique temp directory in Node before spawning
// `go run` — no cleanup needed since every run gets its own directory.
import { spawn } from 'node:child_process'
import { mkdtempSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const backendDir = resolve(__dirname, '../../backend')

const dbDir = mkdtempSync(join(tmpdir(), 'kubepilot-e2e-'))
const dbPath = join(dbDir, 'kubepilot-e2e.db')

const child = spawn('go', ['run', './cmd/server'], {
  cwd: backendDir,
  env: {
    ...process.env,
    DEMO_MODE: 'true',
    PORT: process.env.KUBEPILOT_E2E_PORT || '8090',
    SQLITE_PATH: dbPath,
  },
  stdio: 'inherit',
  // No shell: true — args are static (never user input), and Node resolves
  // `go`/`go.exe` via PATH on Windows without one. Passing args through a
  // shell just adds an unescaped-argument footgun for no benefit here.
})

child.on('exit', (code) => process.exit(code ?? 0))
child.on('error', (err) => {
  console.error('failed to start backend:', err)
  process.exit(1)
})
