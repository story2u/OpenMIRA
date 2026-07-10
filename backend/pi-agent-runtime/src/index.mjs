import process from 'node:process'

import { runAnalysis } from './runtime.mjs'

async function readStdin(maxBytes = 512_000) {
  const chunks = []
  let size = 0
  for await (const chunk of process.stdin) {
    size += chunk.length
    if (size > maxBytes) throw new Error('input exceeds maximum size')
    chunks.push(chunk)
  }
  if (chunks.length === 0) throw new Error('input is required')
  return JSON.parse(Buffer.concat(chunks).toString('utf8'))
}

try {
  const input = await readStdin()
  const result = await runAnalysis(input)
  process.stdout.write(JSON.stringify(result))
} catch (error) {
  const message = error instanceof Error ? error.message : 'unknown error'
  process.stderr.write(`${message}\n`)
  process.exitCode = 1
}
