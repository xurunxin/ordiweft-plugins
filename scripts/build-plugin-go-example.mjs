// SPDX-License-Identifier: Apache-2.0
import {readFile,writeFile,mkdir,lstat,realpath,readdir} from 'node:fs/promises';
import {execFileSync} from 'node:child_process';
import {createHash} from 'node:crypto';
import path from 'node:path';
import {fileURLToPath} from 'node:url';

const repo=path.dirname(path.dirname(fileURLToPath(import.meta.url)));
const builderBytes=await readFile(fileURLToPath(import.meta.url));
const args=process.argv.slice(2),options={};
for(let i=0;i<args.length;i+=2){
 const [key,value]=args.slice(i,i+2);
 if(!['--sdk','--sdk-commit','--out','--example'].includes(key)||!value||options[key])throw Error('usage: --sdk <absolute export> --sdk-commit <40 SHA> --out <new absolute external directory> --example codex-image|gpt-image|indextts|doubao-audio|doubao-speech|dashscope-wan|fish-audio');
 options[key]=value;
}
const example=options['--example'];
const extraSources={
 'codex-image':['payload.go','response.go','main_test.go'],
 'gpt-image':['payload.go','response.go','response_test.go'],
 indextts:[],
 'doubao-audio':[],
 'doubao-speech':['main_test.go'],
 'dashscope-wan':[],
 'fish-audio':[],
};
if(!Object.hasOwn(extraSources,example))throw Error('Unknown Go adapter');
const source=path.join(repo,'examples/plugin-go',example);
const sdk=options['--sdk'],out=options['--out'],commit=options['--sdk-commit'];
if(!path.isAbsolute(sdk??'')||!path.isAbsolute(out??'')||!/^[a-f0-9]{40}$/.test(commit??''))throw Error('absolute SDK/output and fixed SDK commit required');
const below=(parent,child)=>{const relative=path.relative(parent,child);return relative===''||!path.isAbsolute(relative)&&relative!=='..'&&!relative.startsWith('..'+path.sep)};
if(below(repo,out)||below(sdk,out)||below(out,repo)||below(out,sdk))throw Error('output must be separate from source and SDK');
const samePath=(a,b)=>process.platform==='win32'?path.resolve(a).toLowerCase()===path.resolve(b).toLowerCase():path.resolve(a)===path.resolve(b);
if(!samePath(await realpath(sdk),sdk)||!samePath(path.join(await realpath(path.dirname(out)),path.basename(out)),out))throw Error('SDK/output parent must resolve to the named location');
try{await lstat(out);throw Error('output already exists')}catch(e){if(e.code!=='ENOENT')throw e}
if((await lstat(sdk)).isSymbolicLink())throw Error('linked SDK root refused');
const sdkProofBytes=await readFile(path.join(sdk,'sdk-export.json'));
const proof=JSON.parse(sdkProofBytes);
if(proof.version!==1||proof.license!=='Apache-2.0'||proof.sourceCommit!==commit||proof.sourceDirty!==false||!Array.isArray(proof.files))throw Error('clean fixed public SDK export required');
const go=process.platform==='win32'?'go.exe':'go';
const env={};for(const key of ['PATH','SystemRoot','WINDIR','TEMP','TMP','USERPROFILE','LOCALAPPDATA','HOME','GOCACHE'])if(process.env[key])env[key]=process.env[key];
Object.assign(env,{GOENV:'off',GOFLAGS:'',GOTOOLCHAIN:'local',GOPROXY:'off',GOSUMDB:'off',GOWORK:'off',CGO_ENABLED:'0',GOOS:'linux',GOARCH:'amd64',GOAMD64:'v1'});
const goVersion=execFileSync(go,['version'],{env,encoding:'utf8',windowsHide:true}).trim();
if(!goVersion.includes('go'+proof.goVersion+' '))throw Error('installed Go version must equal fixed SDK toolchain');
const goPaths=JSON.parse(execFileSync(go,['env','-json','GOROOT','GOTOOLDIR'],{env,encoding:'utf8',windowsHide:true}));
const toolchainFiles={};
for(const [name,file]of Object.entries({go:path.join(goPaths.GOROOT,'bin',go),compile:path.join(goPaths.GOTOOLDIR,process.platform==='win32'?'compile.exe':'compile'),link:path.join(goPaths.GOTOOLDIR,process.platform==='win32'?'link.exe':'link')}))toolchainFiles[name]=createHash('sha256').update(await readFile(file)).digest('hex');
await mkdir(out);const bundle=path.join(out,'bundle');await mkdir(bundle);
const inventory=[];
async function put(name,bytes){const dest=path.join(bundle,name);await mkdir(path.dirname(dest),{recursive:true});await writeFile(dest,bytes,{flag:'wx'});inventory.push({path:name,bytes:bytes.length,sha256:createHash('sha256').update(bytes).digest('hex')})}
// The Linux/amd64 adapter links code with notices beyond the root Go license.
const goNotices = {
 'Go-LICENSE': 'LICENSE',
 'Go-PATENTS': 'PATENTS',
 'Go-memmove_amd64.s': 'src/runtime/memmove_amd64.s',
 'Go-math-log.go.txt': 'src/math/log.go',
 'Go-fiat-crypto-p256_ordinv.go.txt': 'src/crypto/internal/fips140/nistec/p256_ordinv.go',
 'Go-fiat-crypto-edwards25519-scalar.go.txt': 'src/crypto/internal/fips140/edwards25519/scalar.go',
};
for (const [name, source] of Object.entries(goNotices)) {
 await put('licenses/' + name, await readFile(path.join(goPaths.GOROOT, source)));
}
const names=new Set();
for(const file of proof.files){
 if(typeof file.path!=='string'||file.path.includes('\\')||file.path.startsWith('/')||file.path.split('/').some(p=>!p||p==='.'||p==='..')||names.has(file.path)||!/^[a-f0-9]{64}$/.test(file.sha256)||!Number.isSafeInteger(file.bytes)||file.bytes<0||file.bytes>1048576)throw Error('invalid SDK inventory');
 names.add(file.path);let current=sdk;for(const part of file.path.split('/')){current=path.join(current,part);if((await lstat(current)).isSymbolicLink())throw Error('linked SDK source refused')}
 const bytes=await readFile(current);if(bytes.length!==file.bytes||createHash('sha256').update(bytes).digest('hex')!==file.sha256)throw Error('SDK bytes differ from export');await put('sdk/'+file.path,bytes);
}
for(const required of ['pkg/pluginhost/action_client.go','pkg/pluginhost/asset_client.go','pkg/pluginhost/LICENSE','pkg/pluginhost/NOTICE','pkg/aigc/LICENSE','pkg/aigc/NOTICE','LICENSE','NOTICE','go.mod','.ordiweft-go-sdk-export'])if(!names.has(required))throw Error('required public SDK source/license absent');
await put('sdk/sdk-export.json',sdkProofBytes);
const sourceFiles=['main.go','go.mod','README.md','LICENSE','NOTICE','release.template.json',
 'connection.profile.json',
 ...(await readdir(path.join(source,'fixtures'))).sort().map(name=>'fixtures/'+name),
 ...(await readdir(path.join(source,'policies'))).sort().map(name=>'policies/'+name),
 ...extraSources[example],
];
const sharedPayload = path.join(repo, 'examples/plugin-go/gpt-image/payload.go');
const sourcePaths = [path.relative(repo, source), 'scripts/build-plugin-go-example.mjs'];
if (example === 'codex-image') sourcePaths.push(path.relative(repo, sharedPayload));
for (const name of sourceFiles) {
  const file = example === 'codex-image' && name === 'payload.go' ? sharedPayload : path.join(source, name);
  if (!samePath(await realpath(file), file)) throw Error('linked example source refused');
  await put(name,await readFile(file));
}
const entry='adapter';
execFileSync(go,['build','-trimpath','-buildvcs=false','-ldflags=-buildid=','-o',path.join(bundle,entry),'.'],{cwd:bundle,env,stdio:['ignore','pipe','pipe'],windowsHide:true});
const binary=await readFile(path.join(bundle,entry));inventory.push({path:entry,bytes:binary.length,sha256:createHash('sha256').update(binary).digest('hex')});
const sourceCommit=execFileSync('git',['rev-parse','HEAD'],{cwd:repo,encoding:'utf8',windowsHide:true}).trim();
const sourceDirty=!!execFileSync('git',['status','--porcelain','--',...sourcePaths],{cwd:repo,encoding:'utf8',windowsHide:true}).trim();
await put('build-proof.json',Buffer.from(JSON.stringify({version:1,example,sourceCommit,sourceDirty,builderSha256:createHash('sha256').update(builderBytes).digest('hex'),sdkCommit:commit,sdkExportSha256:createHash('sha256').update(sdkProofBytes).digest('hex'),toolchain:goVersion,toolchainFiles,target:'linux/amd64',cgo:false,execution:'not_run',publication:'not_published',files:[...inventory].sort((a,b)=>a.path<b.path?-1:a.path>b.path?1:0)},null,2)+'\n'));
const release=JSON.parse(await readFile(path.join(bundle,'release.template.json'),'utf8'));
release.files=inventory.sort((a,b)=>a.path<b.path?-1:a.path>b.path?1:0);
await writeFile(path.join(bundle,'release.json'),JSON.stringify(release,null,2)+'\n',{flag:'wx'});

console.log(JSON.stringify({bundle,sourceCommit,sourceDirty,sdkCommit:commit,files:inventory.length,entrySha256:createHash('sha256').update(binary).digest('hex'),execution:'not_run',publication:'not_published'}));
