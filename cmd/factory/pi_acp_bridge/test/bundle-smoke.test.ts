import { spawn } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import { test } from 'node:test'
import assert from 'node:assert/strict'

test('bundled bridge initializes over ACP stdio', async () => {
  const bundle = fileURLToPath(new URL('../dist/index.js', import.meta.url))
  const child = spawn(process.execPath, [bundle], { stdio: ['pipe', 'pipe', 'pipe'] })
  let stderr = ''
  child.stderr.setEncoding('utf8').on('data', chunk => { stderr += chunk })
  try {
    const response = await new Promise<Record<string, unknown>>((resolve, reject) => {
      const timeout = setTimeout(() => reject(new Error(`ACP initialize timed out: ${stderr}`)), 5000)
      let stdout = ''
      child.stdout.setEncoding('utf8').on('data', chunk => {
        stdout += chunk
        const newline = stdout.indexOf('\n')
        if (newline < 0) return
        clearTimeout(timeout)
        try { resolve(JSON.parse(stdout.slice(0, newline))) } catch (error) { reject(error) }
      })
      child.once('exit', code => {
        clearTimeout(timeout)
        reject(new Error(`bridge exited ${code}: ${stderr}`))
      })
      child.stdin.write(JSON.stringify({
        jsonrpc: '2.0', id: 1, method: 'initialize',
        params: { protocolVersion: 1, clientCapabilities: {}, clientInfo: { name: 'bundle-smoke', version: '1' } }
      }) + '\n')
    })
    assert.equal(response.id, 1)
    assert.equal((response.result as { protocolVersion: number }).protocolVersion, 1)
  } finally {
    child.kill()
  }
})
