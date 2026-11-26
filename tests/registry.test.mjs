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
function run(repo,args){return spawnSync(process.execPath,[path.join(repo,'scripts/registry.mjs'),...args],{encoding:'utf8',windowsHide:true,shell:false,timeout:180000})}
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

test('portable repository checks without executing packages or requiring credentials',async()=>{
 const result=run(root,['check'])
 assert.equal(result.status,0,result.stderr)
 assert.deepEqual(JSON.parse(result.stdout),{status:'checked',packages:JSON.parse(await fs.readFile(path.join(root,'repository.json'),'utf8')).packages.length,execution:'not_run',publication:'not_run'})
})

test('declarative worker recipes cannot supply deployment images or endpoints',async t=>{
 const {repo}=await fixture(t)
 const configPath=path.join(repo,'repository.json'),config=JSON.parse(await fs.readFile(configPath))
 const key='ordiweft-examples/png-renderer/1.0.0'
 config.packages=[key];await fs.writeFile(configPath,JSON.stringify(config))
 const recipePath=path.join(repo,'plugins',key,'recipe.json'),recipe=JSON.parse(await fs.readFile(recipePath))
 for(const [extra,message]of [[{runtimeImage:'sha256:'+'a'.repeat(64)},/Declarative recipe cannot select a runtime image/],[{origin:'http://127.0.0.1:49010'},/Unexpected metadata field/]]){
  await fs.writeFile(recipePath,JSON.stringify({...recipe,...extra}))
  const result=run(repo,['check']);assert.notEqual(result.status,0);assert.match(result.stderr,message)
 }
})

test('static process metadata requires an immutable sandbox image without running the package',async t=>{
 const {repo}=await fixture(t)
 const configPath=path.join(repo,'repository.json'),config=JSON.parse(await fs.readFile(configPath))
 const key='ordiweft-examples/go-host-generation/1.0.0'
 config.packages=[key];await fs.writeFile(configPath,JSON.stringify(config))
 const recipePath=path.join(repo,'plugins',key,'recipe.json'),recipe=JSON.parse(await fs.readFile(recipePath))
 delete recipe.runtimeImage;await fs.writeFile(recipePath,JSON.stringify(recipe))
 const result=run(repo,['check'])
 assert.notEqual(result.status,0)
 assert.match(result.stderr,/Executable runtime image must be immutable/)
})

test('Go adapter recipe refuses a source commit that differs from its frozen build',async t=>{
 const {repo}=await fixture(t)
 const recipePath=path.join(repo,'plugins/ordiweft-migrations/fish-audio/1.1.0/recipe.json')
 const recipe=JSON.parse(await fs.readFile(recipePath))
 recipe.source.commit='0'.repeat(40)
 await fs.writeFile(recipePath,JSON.stringify(recipe))
 const result=run(repo,['check'])
 assert.notEqual(result.status,0)
 assert.match(result.stderr,/Go adapter build provenance differs/)
})

test('real host packing and catalog proposals preserve immutable original bytes', {skip:!tool||!toolSHA?'explicit trusted host CLI and SHA-256 required':false}, async t=>{
 const {repo,dir}=await fixture(t)
 const first=path.join(dir,'build-one'),second=path.join(dir,'build-two')
 for(const output of [first,second]){
  if(output===second){
   const config=JSON.parse(await fs.readFile(path.join(repo,'repository.json')))
   for(const key of config.packages){
    const recipe=path.join(repo,'plugins',key,'recipe.json')
    await fs.writeFile(recipe,(await fs.readFile(recipe,'utf8')).replaceAll('\r\n','\n'))
   }
  }
  const result=run(repo,['build',tool,toolSHA,output]);assert.equal(result.status,0,result.stderr)
 }
 const proofPath=path.join(first,'build-proof.json'),proofBody=await fs.readFile(proofPath),proof=JSON.parse(proofBody)
 assert.equal(proof.version,3)
 assert.deepEqual(JSON.parse(await fs.readFile(path.join(second,'build-proof.json'))),proof,'recipe line endings changed portable provenance')
 assert.equal(proof.artifacts.length,JSON.parse(await fs.readFile(path.join(repo,'repository.json'),'utf8')).packages.length)
 assert.ok(proof.artifacts.some(item=>item.source.commit!==proof.hostSource.commit),'mixed original sources lost their provenance')
 for(const artifact of proof.artifacts){
  const bytes=await fs.readFile(path.join(first,artifact.filename))
  assert.equal(sha(bytes),artifact.sha256)
  assert.equal(bytes.length,artifact.bytes)
  assert.deepEqual(bytes,await fs.readFile(path.join(second,artifact.filename)),'same original package must pack identically')
 }
 await verifyNpmCandidates(proof.artifacts,first,dir)
 const selectionPath=path.join(dir,'selection.json'),proposalPath=path.join(dir,'candidate.json')
 const original=proof.artifacts[0]
 const selection={version:1,expectedBuildSHA256:sha(proofBody),decisions:[{key:original.key,state:'candidate',reason:'synthetic integration review only'}]}
 await fs.writeFile(selectionPath,JSON.stringify(selection))
 let result=run(repo,['prepare',proofPath,selectionPath,proposalPath]);assert.equal(result.status,0,result.stderr)
 const candidate=JSON.parse(await fs.readFile(proposalPath))
 assert.equal(candidate.kind,'unsigned-catalog-proposal')
 assert.equal(candidate.trust,'unsigned');assert.equal(candidate.publication,'not_run');assert.equal(candidate.tufSigning,'not_run')
 assert.equal(candidate.entries[0].releaseDigest,original.releaseDigest)
 assert.deepEqual(candidate.entries[0].source,original.source)
 assert.equal(candidate.entries[0].recipeDigest,original.recipeDigest)
 const versions=proof.artifacts.filter(item=>item.packageId==='ordiweft-examples/document-normalize').sort((a,b)=>a.version.localeCompare(b.version))
 assert.deepEqual(versions.map(item=>item.version),['1.0.0','1.0.1'])
 assert.notEqual(versions[0].releaseDigest,versions[1].releaseDigest)
 assert.notDeepEqual(versions[0].source,versions[1].source)
 const versionSelection=path.join(dir,'version-selection.json'),versionCandidate=path.join(dir,'version-candidate.json')
 await fs.writeFile(versionSelection,JSON.stringify({...selection,decisions:versions.map(item=>({key:item.key,state:'candidate',reason:'synthetic original and upgrade review'}))}))
 result=run(repo,['prepare',proofPath,versionSelection,versionCandidate]);assert.equal(result.status,0,result.stderr)
 assert.deepEqual(JSON.parse(await fs.readFile(versionCandidate)).entries.map(item=>item.releaseDigest),versions.map(item=>item.releaseDigest))
 const retainedOld=await fs.readFile(path.join(first,versions[0].filename))
 await fs.writeFile(versionSelection,JSON.stringify({...selection,decisions:[{key:versions[0].key,state:'withdrawn',reason:'synthetic old version withdrawal; new candidate unchanged'},{key:versions[1].key,state:'candidate',reason:'synthetic reviewed upgrade'}]}))
 const versionWithdrawal=path.join(dir,'version-withdrawal.json')
 result=run(repo,['prepare',proofPath,versionSelection,versionWithdrawal]);assert.equal(result.status,0,result.stderr)
 const versionStates=JSON.parse(await fs.readFile(versionWithdrawal)).entries
 assert.deepEqual(versionStates.map(item=>item.state),['withdrawn','candidate'])
 assert.equal(versionStates[1].sha256,versions[1].sha256)
 assert.deepEqual(await fs.readFile(path.join(first,versions[0].filename)),retainedOld,'withdrawing the old version rewrote its immutable original')
 const wrongProvenance=structuredClone(proof)
 wrongProvenance.artifacts[0].source.commit='0'.repeat(40)
 const wrongProofPath=path.join(first,'wrong-provenance.json'),wrongSelectionPath=path.join(dir,'wrong-selection.json')
 const wrongBody=Buffer.from(JSON.stringify(wrongProvenance))
 await fs.writeFile(wrongProofPath,wrongBody)
 await fs.writeFile(wrongSelectionPath,JSON.stringify({...selection,expectedBuildSHA256:sha(wrongBody)}))
 result=run(repo,['prepare',wrongProofPath,wrongSelectionPath,path.join(dir,'wrong-source-proposal.json')])
 assert.notEqual(result.status,0);assert.match(result.stderr,/Original recipe provenance changed/)
 const legacy=structuredClone(proof)
 legacy.version=1;legacy.hostSource={repository:original.source.repository,commit:original.source.commit}
 for(const item of legacy.artifacts){delete item.source;delete item.recipeDigest}
 const legacyPath=path.join(first,'legacy-proof.json'),legacySelectionPath=path.join(dir,'legacy-selection.json')
 let legacyBody=Buffer.from(JSON.stringify(legacy))
 await fs.writeFile(legacyPath,legacyBody)
 await fs.writeFile(legacySelectionPath,JSON.stringify({...selection,expectedBuildSHA256:sha(legacyBody)}))
 result=run(repo,['prepare',legacyPath,legacySelectionPath,path.join(dir,'legacy-candidate.json')]);assert.equal(result.status,0,result.stderr)
 const legacyCandidate=JSON.parse(await fs.readFile(path.join(dir,'legacy-candidate.json')))
 assert.equal(legacyCandidate.entries[0].source.path,undefined,'legacy proof invented an original source path')
 assert.equal(legacyCandidate.entries[0].recipeSHA256,undefined,'legacy proof invented an original recipe fingerprint')
 assert.equal(legacyCandidate.entries[0].recipeDigest,undefined,'legacy proof invented a new recipe fingerprint')
 assert.equal(legacyCandidate.entries[0].sourceEvidence,'legacy-tool-source')
 const newer=proof.artifacts.find(item=>item.source.commit!==original.source.commit)
 await fs.writeFile(legacySelectionPath,JSON.stringify({...selection,expectedBuildSHA256:sha(legacyBody),decisions:[{key:newer.key,state:'candidate',reason:'must not attribute newer source to legacy tool'}]}))
 result=run(repo,['prepare',legacyPath,legacySelectionPath,path.join(dir,'false-legacy-source.json')])
 assert.notEqual(result.status,0);assert.match(result.stderr,/Legacy proof cannot claim a different source/)
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

async function verifyNpmCandidates(artifacts,built,dir){
 const pack=(...args)=>spawnSync(process.execPath,[path.join(root,'scripts/npm-release.mjs'),...args],{encoding:'utf8',windowsHide:true,timeout:15000})
 const seedId='ordiweft-examples/doubao-audio'
 const seed=artifacts.find(item=>item.key===`${seedId}/1.0.3`)
 const bailian=artifacts.find(item=>item.key==='ordiweft-migrations/dashscope/1.1.0')
 const seedZIP=path.join(built,seed.filename), seedOut=path.join(dir,'npm-seedaudio')
 const bailianOut=path.join(dir,'npm-bailian')
 const releases=[
  [[seedId,'1.0.3',seedZIP,seedOut],seedOut,'@daoxialiuren/ordiweft-doubao-audio',seed],
  [['1.1.0',path.join(built,bailian.filename),bailianOut],bailianOut,'@daoxialiuren/ordiweft-dashscope',bailian],
 ]
 for(const [id,version,name]of [
  ['ordiweft-migrations/doubao-speech','1.0.0','@daoxialiuren/ordiweft-doubao-speech'],
  ['ordiweft-examples/indextts','1.0.0','@daoxialiuren/ordiweft-indextts'],
  ['ordiweft-migrations/fish-audio','1.1.0','@daoxialiuren/ordiweft-fish-audio'],
 ]){
  const original=artifacts.find(item=>item.key===`${id}/${version}`)
  const out=path.join(dir,'npm-'+id.split('/')[1])
  releases.push([[id,version,path.join(built,original.filename),out],out,name,original])
 }
 for(const [args,out,name,original]of releases){
  const result=pack(...args)
  assert.equal(result.status,0,result.stderr)
  const metadata=JSON.parse(await fs.readFile(path.join(out,'package.json')))
  assert.equal(metadata.name,name)
  assert.equal(metadata.version,original.version)
  assert.equal(sha(await fs.readFile(path.join(out,'plugin.zip'))),original.sha256)
 }
 const badOut=path.join(dir,'npm-invalid')
 const wrongCandidate=pack(seedId,'1.1.0',seedZIP,badOut)
 assert.notEqual(wrongCandidate.status,0);assert.match(wrongCandidate.stderr,/Select one reviewed npm candidate/)
 const tampered=await fs.readFile(seedZIP)
 tampered[0]^=1
 const tamperedZIP=path.join(dir,'tampered-seedaudio.zip')
 await fs.writeFile(tamperedZIP,tampered)
 const wrongBytes=pack(seedId,'1.0.3',tamperedZIP,badOut)
 assert.notEqual(wrongBytes.status,0);assert.match(wrongBytes.stderr,/Candidate ZIP SHA-256 differs/)
 await assert.rejects(fs.access(badOut),{code:'ENOENT'})
 const repeated=pack(seedId,'1.0.3',seedZIP,seedOut)
 assert.notEqual(repeated.status,0);assert.match(repeated.stderr,/EEXIST/)
 assert.equal(sha(await fs.readFile(path.join(seedOut,'plugin.zip'))),seed.sha256)
}

test('actual host verifier rejects changed source; wrong CLI digest never builds', {skip:!tool||!toolSHA?'explicit trusted host CLI and SHA-256 required':false}, async t=>{
 const {repo,dir}=await fixture(t)
 let result=run(repo,['build',tool,'0'.repeat(64),path.join(dir,'wrong-tool')])
 assert.notEqual(result.status,0);assert.match(result.stderr,/CLI digest mismatch/)
 await fs.appendFile(path.join(repo,'plugins/ordiweft-examples/typed-content/1.0.0/source/title.json'),' ')
 result=run(repo,['build',tool,toolSHA,path.join(dir,'changed-source')])
 assert.notEqual(result.status,0);assert.match(result.stderr,/Trusted host tool rejected bundle/)
})
