import {createHash} from 'node:crypto'
import {copyFile, lstat, mkdir, readFile, realpath, writeFile} from 'node:fs/promises'
import path from 'node:path'
import {fileURLToPath} from 'node:url'

const root = fileURLToPath(new URL('../', import.meta.url))
const repositoryURL = 'https://github.com/xurunxin/ordiweft-plugins'
const registry = 'https://registry.npmjs.org'
const sha256 = bytes => createHash('sha256').update(bytes).digest('hex')

async function prepare(version, archivePath, destination) {
  const {entries} = JSON.parse(await readFile(path.join(root, 'repository/npm-candidates.json'), 'utf8'))
  const candidates = entries.filter(entry => entry.version === version && entry.packageId === 'ordiweft-migrations/dashscope')
  if (candidates.length !== 1) throw new Error('Select one reviewed Bailian candidate.')
  const entry = candidates[0]
  const zip = await lstat(archivePath)
  if (!zip.isFile() || zip.size !== entry.bytes) throw new Error('Candidate ZIP length or type differs.')
  const bytes = await readFile(archivePath)
  if (sha256(bytes) !== entry.sha256) throw new Error('Candidate ZIP SHA-256 differs.')

  const parent = await realpath(path.dirname(path.resolve(destination)))
  const out = path.join(parent, path.basename(destination))
  const relative = path.relative(await realpath(root), out)
  if (!relative.startsWith('..' + path.sep) && !path.isAbsolute(relative)) {
    throw new Error('Use a fresh destination outside the repository.')
  }
  await mkdir(out)
  const packageJSON = {
    name: entry.npmPackage,
    version: entry.version,
    description: 'Prebuilt Bailian channel plugin for Ordiweft',
    license: 'Apache-2.0',
    repository: {type: 'git', url: 'git+' + repositoryURL + '.git'},
    files: ['plugin.zip', 'NOTICE'],
    publishConfig: {access: 'public', registry}
  }
  const source = path.join(root, 'plugins', entry.packageId, entry.version, 'source')
  await copyFile(path.join(source, 'LICENSE'), path.join(out, 'LICENSE'))
  await copyFile(path.join(source, 'NOTICE'), path.join(out, 'NOTICE'))
  await writeFile(path.join(out, 'package.json'), JSON.stringify(packageJSON, null, 2) + '\n')
  await writeFile(path.join(out, 'plugin.zip'), bytes)
  console.log(JSON.stringify({package: entry.npmPackage, version, sha256: entry.sha256}))
}

try {
  const args = process.argv.slice(2)
  if (args.length !== 3) throw new Error('Usage: node scripts/npm-release.mjs <version> <original ZIP> <new package directory>')
  await prepare(...args)
} catch (error) {
  console.error(error.message)
  process.exitCode = 1
}
