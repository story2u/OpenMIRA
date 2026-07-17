import { readdirSync, readFileSync } from 'node:fs';
import { extname, join } from 'node:path';
import { expect, it } from 'vitest';

const roots = [join(process.cwd(), 'app'), join(process.cwd(), 'src')];
const serverTemplateTokens = /\{\{(?:联系人姓名|群名称|公司名称|工作时间)\}\}/g;

function productionScreens(directory: string): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) return productionScreens(path);
    return extname(path) === '.tsx' && !entry.name.includes('.test.') ? [path] : [];
  });
}

it('keeps user-facing Chinese screen copy in the typed catalog', () => {
  const offenders = roots.flatMap(productionScreens).filter((path) => {
    const source = readFileSync(path, 'utf8').replace(serverTemplateTokens, '');
    return /[\u3400-\u9fff]/.test(source);
  });
  expect(offenders).toEqual([]);
});
