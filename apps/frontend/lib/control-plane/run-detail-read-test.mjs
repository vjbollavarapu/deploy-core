import { spawnSync } from 'node:child_process'
import fs from 'node:fs'
import { createRequire } from 'node:module'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const require = createRequire(import.meta.url)
const ts = require('typescript')

const here = path.dirname(fileURLToPath(import.meta.url))
const out = fs.mkdtempSync(path.join(os.tmpdir(), 'dc-detail-read-'))

function emit(filename) {
  const source = fs
    .readFileSync(path.join(here, filename), 'utf8')
    .replaceAll("from './detail-read'", "from './detail-read.js'")
  const js = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.ES2022,
      target: ts.ScriptTarget.ES2022,
    },
  }).outputText
  fs.writeFileSync(path.join(out, filename.replace(/\.ts$/, '.js')), js)
}

emit('detail-read.ts')
emit('detail-read.test.ts')

const result = spawnSync(process.execPath, ['--test', path.join(out, 'detail-read.test.js')], {
  stdio: 'inherit',
})
process.exit(result.status ?? 1)
