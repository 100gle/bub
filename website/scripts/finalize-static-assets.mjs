import { rename, rm } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const websiteDir = fileURLToPath(new URL('..', import.meta.url));
const localized404Dir = path.join(websiteDir, 'dist/zh-cn/404');
const source = path.join(localized404Dir, 'index.html');
const target = path.join(websiteDir, 'dist/zh-cn/404.html');

await rename(source, target);
await rm(localized404Dir, { recursive: true });
