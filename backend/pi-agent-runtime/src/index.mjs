import process from 'node:process'

import { runAnalysis, runPreferenceParse, runSourceProfile } from './runtime.mjs'

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
  let runtimeMeta = null
  const options = { onMetrics: (metrics) => { runtimeMeta = metrics } }
  const result = input?.task === 'parse_job_search_profile'
    ? await runPreferenceParse(input, options)
    : input?.task === 'profile_source_function'
      ? await runSourceProfile(input, options)
      : await runAnalysis(input, options)
  process.stdout.write(JSON.stringify({ result, runtime_meta: runtimeMeta }))
} catch (error) {
  const message = error instanceof Error ? error.message : 'unknown error'
  process.stderr.write(`${message}\n`)
  process.exitCode = 1
}
