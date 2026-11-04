import fs from 'node:fs/promises';
import path from 'node:path';
import { randomUUID } from 'node:crypto';

// Write in the destination directory, then rename. A failed write leaves the
// previous complete file intact; callers must still return a nonzero exit code.
export async function writeTextAtomic(file, data) {
  const temp = path.join(path.dirname(file), `.${path.basename(file)}.${randomUUID()}.tmp`);
  try {
    await fs.writeFile(temp, data, { encoding: 'utf8', flag: 'wx' });
    await fs.rename(temp, file);
  } finally {
    await fs.rm(temp, { force: true });
  }
}
