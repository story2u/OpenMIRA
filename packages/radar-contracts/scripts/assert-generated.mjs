import { readFile, rm } from 'node:fs/promises';

const generatedPath = new URL('../.generated/openapi.ts', import.meta.url);
const committedPath = new URL('../src/openapi.ts', import.meta.url);

const [generated, committed] = await Promise.all([
  readFile(generatedPath, 'utf8'),
  readFile(committedPath, 'utf8'),
]);
await rm(new URL('../.generated', import.meta.url), { force: true, recursive: true });

if (generated !== committed) {
  process.stderr.write('Generated TypeScript contracts are stale. Run pnpm contracts:generate.\n');
  process.exitCode = 1;
}
