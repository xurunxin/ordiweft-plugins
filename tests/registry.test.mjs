import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'
import crypto from 'node:crypto'
import {spawnSync} from 'node:child_process'
import {fileURLToPath} from 'node:url'

const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..')
const sha=bytes=>crypto.createHash('sha256').update(bytes).digest('hex')
const tool=process.env.ORDIWEFT_PLUGIN_CLI,toolSHA=process.env.ORDIWEFT_PLUGIN_CLI_SHA256
function run(repo,args){return spawnSync(process.execPath,[path.join(repo,'scripts/registry.mjs'),...args],{encoding:'utf8',windowsHide:true,shell:false,timeout:60000})}
async function fixture(t){
 const dir=await fs.mkdtemp(path.join(os.tmpdir(),'ordiweft-community-'))
 t.after(async()=>{
  assert.equal(await fs.realpath(dir),dir,'temporary root changed ownership')
  assert.equal(path.dirname(dir),await fs.realpath(os.tmpdir()),'temporary root escaped parent')
  await fs.rm(dir,{recursive:true,force:true})
 })
 const repo=path.join(dir,'repo');await fs.mkdir(repo)
 for(const name of ['repository.json','publishers','plugins','scripts'])await fs.cp(path.join(root,name),path.join(repo,name),{recursive:true,errorOnExist:true,force:false})
 return {repo,dir}
}

test('portable repository checks without executing packages or requiring credentials',()=>{
 const result=run(root,['check'])
 assert.equal(result.status,0,result.stderr)
 assert.deepEqual(JSON.parse(result.stdout),{status:'checked',packages:4,execution:'not_run',publication:'not_run'})
})

test('real host packing and catalog proposals preserve immutable original bytes', {skip:!tool||!toolSHA?'explicit trusted host CLI and SHA-256 required':false}, async t=>{
 const {repo,dir}=await fixture(t)
 const first=path.join(dir,'build-one'),second=path.join(dir,'build-two')
 for(const output of [first,second]){const result=run(repo,['build',tool,toolSHA,output]);assert.equal(result.status,0,result.stderr)}
 const proofPath=path.join(first,'build-proof.json'),proofBody=await fs.readFile(proofPath),proof=JSON.parse(proofBody)
 assert.equal(proof.artifacts.length,4)
 for(const artifact of proof.artifacts){
  const bytes=await fs.readFile(path.join(first,artifact.filename))
  assert.equal(sha(bytes),artifact.sha256)
  assert.equal(bytes.length,artifact.bytes)
  assert.deepEqual(bytes,await fs.readFile(path.join(second,artifact.filename)),'same original package must pack identically')
 }
 const selectionPath=path.join(dir,'selection.json'),proposalPath=path.join(dir,'candidate.json')
 const original=proof.artifacts[0]
 const selection={version:1,expectedBuildSHA256:sha(proofBody),decisions:[{key:original.key,state:'candidate',reason:'synthetic integration review only'}]}
 await fs.writeFile(selectionPath,JSON.stringify(selection))
 let result=run(repo,['prepare',proofPath,selectionPath,proposalPath]);assert.equal(result.status,0,result.stderr)
 const candidate=JSON.parse(await fs.readFile(proposalPath))
 assert.equal(candidate.kind,'unsigned-catalog-proposal')
 assert.equal(candidate.trust,'unsigned');assert.equal(candidate.publication,'not_run');assert.equal(candidate.tufSigning,'not_run')
 assert.equal(candidate.entries[0].releaseDigest,original.releaseDigest)
 const before=await fs.readFile(proposalPath)
 result=run(repo,['prepare',proofPath,selectionPath,proposalPath]);assert.notEqual(result.status,0)
 assert.deepEqual(await fs.readFile(proposalPath),before,'existing proposal overwritten')
 selection.decisions[0].state='withdrawn';selection.decisions[0].reason='synthetic withdrawal without deletion'
 await fs.writeFile(selectionPath,JSON.stringify(selection))
 const withdrawalPath=path.join(dir,'withdrawal.json')
 result=run(repo,['prepare',proofPath,selectionPath,withdrawalPath]);assert.equal(result.status,0,result.stderr)
 const withdrawal=JSON.parse(await fs.readFile(withdrawalPath))
 assert.equal(withdrawal.entries[0].state,'withdrawn');assert.equal(withdrawal.entries[0].sha256,original.sha256)
 assert.equal(sha(await fs.readFile(path.join(first,original.filename))),original.sha256,'withdrawal removed or rewrote artifact')
 selection.decisions[0].state='stable';await fs.writeFile(selectionPath,JSON.stringify(selection))
 result=run(repo,['prepare',proofPath,selectionPath,path.join(dir,'stable.json')]);assert.notEqual(result.status,0)
 selection.decisions[0].state='candidate';selection.expectedBuildSHA256='0'.repeat(64);await fs.writeFile(selectionPath,JSON.stringify(selection))
 result=run(repo,['prepare',proofPath,selectionPath,path.join(dir,'wrong-build.json')]);assert.notEqual(result.status,0)
 selection.expectedBuildSHA256=sha(proofBody);await fs.writeFile(selectionPath,JSON.stringify(selection))
 await fs.appendFile(path.join(first,original.filename),'changed')
 result=run(repo,['prepare',proofPath,selectionPath,path.join(dir,'changed-artifact.json')]);assert.notEqual(result.status,0)
 assert.match(result.stderr,/Artifact bytes changed/)
})

test('actual host verifier rejects changed source; wrong CLI digest never builds', {skip:!tool||!toolSHA?'explicit trusted host CLI and SHA-256 required':false}, async t=>{
 const {repo,dir}=await fixture(t)
 let result=run(repo,['build',tool,'0'.repeat(64),path.join(dir,'wrong-tool')])
 assert.notEqual(result.status,0);assert.match(result.stderr,/CLI digest mismatch/)
 await fs.appendFile(path.join(repo,'plugins/ordiweft-examples/typed-content/1.0.0/source/title.json'),' ')
 result=run(repo,['build',tool,toolSHA,path.join(dir,'changed-source')])
 assert.notEqual(result.status,0);assert.match(result.stderr,/Trusted host tool rejected bundle/)
})
