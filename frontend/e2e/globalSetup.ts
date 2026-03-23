import { execSync, spawn } from 'child_process'
import { mkdtempSync, rmSync } from 'fs'
import { tmpdir } from 'os'
import { dirname, join } from 'path'
import { fileURLToPath } from 'url'

const __dirname = dirname(fileURLToPath(import.meta.url))

export const BACKEND_PORT = 18081
const GRPC_PORT = 18052

export default async function globalSetup() {
  const backendDir = join(__dirname, '../../backend')
  const binaryPath = join(tmpdir(), 'ng-backend-e2e')
  const dataDir = mkdtempSync(join(tmpdir(), 'ng-e2e-'))

  console.log('[e2e] Building backend...')
  execSync(`go build -o ${binaryPath} .`, { cwd: backendDir, stdio: 'inherit' })

  console.log('[e2e] Starting backend...')
  const proc = spawn(binaryPath, [
    '--dir', dataDir,
    '--port', String(GRPC_PORT),
    '--http-port', String(BACKEND_PORT),
  ])
  proc.stderr.on('data', (d: Buffer) => process.stderr.write(d))

  await waitForURL(`http://localhost:${BACKEND_PORT}/api/v1/projects`)
  console.log('[e2e] Backend ready')

  return async function teardown() {
    proc.kill('SIGTERM')
    rmSync(dataDir, { recursive: true, force: true })
    try { rmSync(binaryPath) } catch { /* ignore */ }
  }
}

async function waitForURL(url: string, timeoutMs = 30_000) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      const res = await fetch(url)
      if (res.ok) return
    } catch { /* not ready yet */ }
    await new Promise(r => setTimeout(r, 200))
  }
  throw new Error(`Backend did not become ready at ${url} within ${timeoutMs}ms`)
}
