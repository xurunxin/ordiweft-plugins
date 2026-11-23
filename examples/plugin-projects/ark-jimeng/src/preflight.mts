import type {MediaInfo,TaskInput} from '@ordiweft/plugin-host';

type Facts=MediaInfo&{transparent?:boolean};
export function validateInputs(g:Record<string,unknown>,inputs:TaskInput[],facts:Facts[]){
 const p=(g.parameters??{}) as Record<string,unknown>,model=String(g.model),prompt=String(g.prompt??'').trim();
 if(facts.length!==inputs.length)throw Error('Original input inspection missing');
 for(const [i,input] of inputs.entries())if(facts[i]!.sha256!==input.asset.sha256||facts[i]!.bytes!==input.asset.bytes)throw Error('Original input bytes changed');
 if(g.capability==='image.generate'){
  if(!p.layer_decomposition&&!prompt)throw Error('请填写图片描述');
  for(const [i,input] of inputs.entries()){
   const f=facts[i]!,w=f.width??0,h=f.height??0;
   if(input.kind!=='image'||!['image/png','image/jpeg','image/webp','image/gif'].includes(f.mime)||f.bytes>=30*2**20||w<=14||h<=14||w*h>36000000||w/h<1/16||w/h>16)throw Error('Seedream 参考图尺寸或格式无效');
  }
  if(p.layer_decomposition===true&&(inputs.length!==1||!['image/png','image/jpeg'].includes(facts[0]?.mime??'')||(facts[0]?.width??0)*(facts[0]?.height??0)<262144))throw Error('拆层须连接至少 512×512 总像素的 PNG/JPEG 图片');
  if(p.background==='transparent'){
   if(p.output_format!=='png')throw Error('透明背景须选择 PNG');
   if(model!=='doubao-seedream-5-5-261028'&&(inputs.length!==1||facts[0]?.transparent!==true))throw Error('透明输出须使用已确认带透明像素的参考图');
  }
  const options=p.sequential_image_generation_options as {max_images?:number}|undefined;
  if(options&&Number(options.max_images)+inputs.length>15)throw Error('参考图与最多生成图片总数不能超过 15');
  return;
 }
 if(p.sourceJob){if(inputs.length||prompt)throw Error('正式视频沿用原样片内容');return;}
 if(!inputs.length&&!prompt)throw Error('文生视频须填写描述');
 const mode=p.input_mode,max=model.includes('2-5-')||model==='seedance-2.5-auto'?30:15;
 if(mode==='reference'&&!inputs.length)throw Error('全模态参考须连接素材');
 if(max===15&&inputs.length&&inputs.every(x=>x.kind==='audio'))throw Error('Seedance 2.0 不支持纯音频参考');
 let videoMS=0,audioMS=0,encodedBytes=0;
 for(const [i,input] of inputs.entries()){
  const f=facts[i]!,w=f.width??0,h=f.height??0,ms=f.durationMs??0;
  if(input.kind!=='video')encodedBytes+=4*Math.ceil(f.bytes/3)+128;
  if(encodedBytes>63*2**20)throw Error('图片与音频编码后超过请求容量');
  if(input.kind==='image'){
   if(!['image/png','image/jpeg','image/webp','image/gif'].includes(f.mime)||f.bytes>=30*2**20||w<300||h<300||w>6000||h>6000||w/h<0.4||w/h>2.5)throw Error('Seedance 参考图尺寸或格式无效');
  }else if(input.kind==='audio'){
   if(!['audio/wav','audio/mpeg'].includes(f.mime)||f.bytes>15000000||ms<2000||ms>max*1000)throw Error('Seedance 音频格式、容量或时长无效');
   audioMS+=ms;
  }else if(input.kind==='video'){
   if(f.mime!=='video/mp4'||f.bytes>200000000||ms<(max===30&&mode==='edit'?4000:2000)||ms>max*1000||w<300||h<300||w>6000||h>6000||w*h<407696||w*h>8295044||w/h<0.4||w/h>2.5)throw Error('Seedance 视频尺寸、容量或时长无效');
   videoMS+=ms;
  }else throw Error('不支持的参考素材');
 }
 if(videoMS>max*1000+1||audioMS>max*1000+1)throw Error('参考素材总时长超过模型上限');
}
