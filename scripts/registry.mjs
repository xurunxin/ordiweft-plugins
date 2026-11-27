import fs from 'node:fs/promises'
import path from 'node:path'
import crypto from 'node:crypto'
import {spawnSync} from 'node:child_process'
import {fileURLToPath} from 'node:url'

const sha=bytes=>crypto.createHash('sha256').update(bytes).digest('hex')
const hex=/^[a-f0-9]{64}$/
const identity=/^[a-z][a-z0-9-]{0,62}\/[a-z][a-z0-9-]{0,62}\/[0-9]+\.[0-9]+\.[0-9]+(?:-[a-zA-Z0-9.-]+)?$/
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..')

function requireValue(test,message){if(!test)throw Error(message)}
async function regular(file,max=32<<20){
 const stat=await fs.lstat(file)
 requireValue(stat.isFile()&&!stat.isSymbolicLink()&&stat.size<=max,'Expected bounded regular file')
 return fs.readFile(file)
}
async function anchored(relative){
 requireValue(typeof relative==='string'&&!relative.includes('\\')&&!path.isAbsolute(relative)&&relative.split('/').every(part=>part&&part!=='.'&&part!=='..'),'Invalid repository path')
 const file=path.resolve(root,relative)
 requireValue((await fs.realpath(file))===file,'Linked repository path refused')
 return file
}
async function json(relative){return JSON.parse((await regular(await anchored(relative),256<<10)).toString('utf8'))}
function exact(value,keys){requireValue(value&&typeof value==='object'&&!Array.isArray(value)&&Object.keys(value).every(key=>keys.includes(key)),'Unexpected metadata field')}
function pinnedHost(value){
 exact(value,['repository','commit'])
 requireValue(value.repository==='https://github.com/xurunxin/Ordiweft'&&/^[a-f0-9]{40}$/.test(value.commit),'Pinned host source required')
}
function sameSource(a,b){return a?.repository===b?.repository&&a?.commit===b?.commit&&a?.path===b?.path}
async function repository(){
 const config=await json('repository.json')
 exact(config,['formatVersion','status','license','publication','hostSource','retainedHostSources','packages'])
 requireValue(config.formatVersion===1&&config.status==='unpublished'&&config.publication==='manual-tuf'&&config.license==='Apache-2.0','Unsupported repository policy')
 pinnedHost(config.hostSource)
 requireValue(config.retainedHostSources===undefined||Array.isArray(config.retainedHostSources)&&config.retainedHostSources.length<=32,'Invalid retained tool sources')
 for(const source of config.retainedHostSources??[])pinnedHost(source)
 requireValue(Array.isArray(config.packages)&&config.packages.length<=10000&&new Set(config.packages).size===config.packages.length,'Invalid package index')
 const packages=[]
 for(const key of config.packages){
  requireValue(typeof key==='string'&&identity.test(key),'Invalid package identity')
  const [publisher,name,version]=key.split('/')
  const owner=await json(`publishers/${publisher}/owner.json`)
  exact(owner,['version','publisher','repositoryOwner','status'])
  requireValue(owner.version===1&&owner.publisher===publisher&&owner.status==='pending'&&owner.repositoryOwner==='xurunxin','Invalid pending namespace record')
  const recipeBody=await regular(await anchored(`plugins/${key}/recipe.json`),256<<10),recipe=JSON.parse(recipeBody.toString('utf8'))
  exact(recipe,['version','packageId','releaseVersion','mode','source','runtimeImage'])
  requireValue(recipe.version===1&&recipe.mode==='portable-pack'&&recipe.packageId===`${publisher}/${name}`&&recipe.releaseVersion===version,'Invalid portable recipe')
  exact(recipe.source,['repository','commit','path'])
  const hostExample=recipe.source.repository===config.hostSource.repository&&typeof recipe.source.path==='string'&&recipe.source.path.startsWith('examples/')&&recipe.source.path.split('/').every(part=>part&&part!=='.'&&part!=='..'&&!part.includes('\\'))
  const maintainedInRepository=recipe.source.repository==='https://github.com/xurunxin/ordiweft-plugins'
  const bailianSource=maintainedInRepository&&recipe.packageId==='ordiweft-migrations/dashscope'&&['native/dashscope','executors/dashscope'].includes(recipe.source.path)
  const goAdapterSource=maintainedInRepository&&publisher==='ordiweft-migrations'&&
   ['doubao-speech','fish-audio'].includes(name)&&recipe.source.path===`examples/plugin-go/${name}`
  const channelProfile = maintainedInRepository && publisher === 'ordiweft-migrations' &&
   (['tapnow','tapnow-web'].includes(name) && recipe.source.path === 'native/'+name ||
    ['breeze-tts-api','breeze-tts-local'].includes(name) && recipe.source.path === 'native/breeze-tts' ||
    ['higgsfield','apimart'].includes(name) && recipe.source.path === 'native/'+name)
  const comfyLibrary = maintainedInRepository && recipe.packageId === 'ordiweft-migrations/comfyui' && recipe.source.path === 'native/comfyui'
  requireValue((hostExample||bailianSource||goAdapterSource||channelProfile||comfyLibrary)&&/^[a-f0-9]{40}$/.test(recipe.source.commit),'Unpinned source recipe')
  const source=await anchored(`plugins/${key}/source`)
  const descriptor=JSON.parse((await regular(path.join(source,'release.json'),1<<20)).toString('utf8'))
  requireValue(descriptor.packageId===recipe.packageId&&descriptor.version===version&&descriptor.license==='Apache-2.0','Recipe and package differ')
  if(bailianSource){
   const executable=recipe.source.path==='executors/dashscope'
   requireValue(descriptor.runtime===(executable?'process':'data')&&descriptor.providerProfile===(executable?'dashscope-exec-v1':'dashscope-v1'),'Bailian profile contract differs')
  }
  if(goAdapterSource){
   const build=await json(`plugins/${key}/source/build-proof.json`)
   requireValue(descriptor.runtime==='process'&&descriptor.generation?.id===name&&
    build.example===name&&build.sourceCommit===recipe.source.commit&&build.sourceDirty===false&&
    /^[a-f0-9]{40}$/.test(build.sdkCommit)&&build.target==='linux/amd64'&&build.cgo===false,'Go adapter build provenance differs')
  }
  if (channelProfile) {
   const profile = {'native/breeze-tts':'breeze-stdio-v1','native/higgsfield':'higgsfield-stdio-v1','native/apimart':'apimart-stdio-v1'}[recipe.source.path] ?? 'tapnow-host-v1'
   requireValue(descriptor.runtime === 'data' && descriptor.generation?.id === name &&
    descriptor.providerProfile === profile, 'Channel profile contract differs')
   if (profile !== 'tapnow-host-v1') {
    const build = await json(`plugins/${key}/source/build-proof.txt`)
    requireValue(build.version === 1 && build.sourceCommit === recipe.source.commit &&
     /^[a-f0-9]{40}$/.test(build.sdkCommit) && build.adapters?.length === 2, 'Stdio build provenance differs')
    for (const adapter of ['bin/linux-amd64', 'bin/windows-amd64.exe']) {
     const file = build.adapters.find(file => file.path === adapter)
     const declared = descriptor.files.find(file => file.path === adapter)
     requireValue(file && declared?.sha256 === file.sha256 && declared.bytes === file.bytes, 'Stdio adapter inventory differs')
    }
   }
  }
  if (comfyLibrary) requireValue(descriptor.runtime === 'data' && descriptor.providerProfile === 'comfy-workflows-v1' && descriptor.generation?.id === name,'ComfyUI library contract differs')
  requireValue(['data','node','process','worker'].includes(descriptor.runtime),'Runtime not installed by portable recipes')
  if(descriptor.runtime==='node'||descriptor.runtime==='process')requireValue(typeof recipe.runtimeImage==='string'&&/^sha256:[a-f0-9]{64}$/.test(recipe.runtimeImage),'Executable runtime image must be immutable')
  else requireValue(recipe.runtimeImage===undefined,'Declarative recipe cannot select a runtime image')
  const recipeSHA256=sha(recipeBody)
  const recipeDigest=sha(Buffer.from(JSON.stringify(recipe),'utf8'))
  packages.push({key,recipe,recipeSHA256,recipeDigest,source})
 }
 return {config,packages}
}
async function newDirectory(target){
 const absolute=path.resolve(target)
 const parent=path.dirname(absolute)
 requireValue((await fs.realpath(parent))===parent,'Output parent must be a real directory')
 const relative=path.relative(root,absolute)
 requireValue(relative.startsWith('..'+path.sep)||path.isAbsolute(relative),'Output must be outside the repository')
 await fs.mkdir(absolute,{recursive:false,mode:0o700})
 return absolute
}
function host(tool,args){
 const output=spawnSync(tool,args,{shell:false,windowsHide:true,encoding:'utf8',timeout:30000,maxBuffer:1<<20,env:Object.fromEntries(['PATH','SystemRoot','WINDIR','TEMP','TMP'].filter(key=>process.env[key]).map(key=>[key,process.env[key]]))})
 requireValue(!output.error&&output.status===0,'Trusted host tool rejected bundle')
 const result=JSON.parse(output.stdout)
 requireValue(hex.test(result.digest)&&result.trust==='not_checked'&&result.execution==='not_run','Unexpected host tool response')
 return result
}
async function build(tool,expected,target){
 requireValue(path.isAbsolute(tool)&&hex.test(expected),'Absolute trusted host CLI and SHA-256 required')
 requireValue((await fs.realpath(tool))===path.resolve(tool)&&sha(await regular(tool,128<<20))===expected,'Trusted host CLI digest mismatch')
 const {config,packages}=await repository()
 const destination=await newDirectory(target)
 const artifacts=[]
 for(const entry of packages){
  const verified=host(tool,['verify','-root',entry.source])
  requireValue(verified.packageId===entry.recipe.packageId&&verified.version===entry.recipe.releaseVersion,'Host tool identity mismatch')
  const filename=entry.key.replaceAll('/','--')+'.zip',file=path.join(destination,filename)
  const packed=host(tool,['pack','-root',entry.source,'-out',file])
  requireValue(packed.digest===verified.digest,'Packing changed release')
  const bytes=await regular(file)
  artifacts.push({key:entry.key,packageId:verified.packageId,version:verified.version,releaseDigest:verified.digest,filename,sha256:sha(bytes),bytes:bytes.length,runtimeImage:entry.recipe.runtimeImage??null,source:entry.recipe.source,recipeDigest:entry.recipeDigest})
 }
 const proof={version:3,hostSource:config.hostSource,hostToolSHA256:expected,artifacts,execution:'not_run',trust:'unsigned',publication:'not_run'}
 await fs.writeFile(path.join(destination,'build-proof.json'),JSON.stringify(proof,null,2)+'\n',{flag:'wx',mode:0o600})
 return proof
}
async function prepare(proofPath,selectionPath,target){
 const proofBody=await regular(path.resolve(proofPath),1<<20),proof=JSON.parse(proofBody)
 const selection=JSON.parse((await regular(path.resolve(selectionPath),256<<10)).toString('utf8'))
 exact(selection,['version','expectedBuildSHA256','decisions'])
 requireValue(selection.version===1&&selection.expectedBuildSHA256===sha(proofBody)&&Array.isArray(selection.decisions)&&selection.decisions.length<=10000,'Selection must bind original build proof')
 const {config,packages}=await repository()
 requireValue([1,2,3].includes(proof.version)&&proof.trust==='unsigned'&&proof.execution==='not_run'&&hex.test(proof.hostToolSHA256)&&[config.hostSource,...config.retainedHostSources??[]].some(source=>sameSource(source,proof.hostSource))&&Array.isArray(proof.artifacts),'Invalid original build proof')
 const seen=new Set(),entries=[]
 for(const decision of selection.decisions){
  exact(decision,['key','state','reason'])
  requireValue(identity.test(decision.key)&&!seen.has(decision.key)&&['candidate','withdrawn'].includes(decision.state)&&typeof decision.reason==='string'&&decision.reason.length>0&&decision.reason.length<=512,'Invalid catalog decision')
  seen.add(decision.key)
  const artifact=proof.artifacts.find(item=>item.key===decision.key)
  requireValue(artifact&&config.packages.includes(decision.key)&&hex.test(artifact.sha256)&&hex.test(artifact.releaseDigest)&&Number.isSafeInteger(artifact.bytes),'Original built artifact required')
  requireValue(proof.artifacts.filter(item=>item.key===decision.key).length===1,'Ambiguous original artifact')
  const entry=packages.find(item=>item.key===decision.key)
  requireValue(artifact.packageId===entry.recipe.packageId&&artifact.version===entry.recipe.releaseVersion&&(artifact.runtimeImage??null)===(entry.recipe.runtimeImage??null),'Original recipe identity changed')
  if(proof.version>=2)requireValue(sameSource(artifact.source,entry.recipe.source)&&(proof.version===2?artifact.recipeSHA256===entry.recipeSHA256:artifact.recipeDigest===entry.recipeDigest),'Original recipe provenance changed')
  else requireValue(artifact.source===undefined&&artifact.recipeSHA256===undefined&&artifact.recipeDigest===undefined&&entry.recipe.source.repository===proof.hostSource.repository&&entry.recipe.source.commit===proof.hostSource.commit,'Legacy proof cannot claim a different source')
  requireValue(path.basename(artifact.filename)===artifact.filename,'Invalid artifact name')
  const bytes=await regular(path.join(path.dirname(path.resolve(proofPath)),artifact.filename))
  requireValue(bytes.length===artifact.bytes&&sha(bytes)===artifact.sha256,'Artifact bytes changed after build')
  const provenance=proof.version>=2?{source:artifact.source,...(proof.version===2?{recipeSHA256:artifact.recipeSHA256}:{recipeDigest:artifact.recipeDigest})}:{source:{repository:proof.hostSource.repository,commit:proof.hostSource.commit},sourceEvidence:'legacy-tool-source'}
  entries.push({...artifact,...provenance,state:decision.state,reason:decision.reason})
 }
 entries.sort((a,b)=>a.key.localeCompare(b.key,'en'))
 const proposal={version:1,kind:'unsigned-catalog-proposal',buildSHA256:sha(proofBody),entries,trust:'unsigned',publisherApproval:'required',tufSigning:'not_run',publication:'not_run'}
 await fs.writeFile(path.resolve(target),JSON.stringify(proposal,null,2)+'\n',{flag:'wx',mode:0o600})
 return proposal
}
try{
 const [command,...args]=process.argv.slice(2)
 if(command==='check'&&args.length===0){const value=await repository();console.log(JSON.stringify({status:'checked',packages:value.packages.length,execution:'not_run',publication:'not_run'}))}
 else if(command==='build'&&args.length===3){const proof=await build(...args);console.log(JSON.stringify({status:'packed',artifacts:proof.artifacts.length,trust:proof.trust,execution:proof.execution,publication:proof.publication}))}
 else if(command==='prepare'&&args.length===3){const proposal=await prepare(...args);console.log(JSON.stringify({status:'prepared',entries:proposal.entries.length,trust:proposal.trust,publication:proposal.publication}))}
 else throw Error('Usage: node scripts/registry.mjs check | build <absolute-host-cli> <sha256> <new-external-directory> | prepare <build-proof.json> <selection.json> <new-proposal.json>')
}catch(error){console.error(error.message);process.exitCode=1}
