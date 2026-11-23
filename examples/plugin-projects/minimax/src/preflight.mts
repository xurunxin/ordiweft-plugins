interface Input {kind:string;role?:string;asset:{bytes:number;sha256:string}}
interface Fact {mime?:unknown;width?:unknown;height?:unknown;durationMs?:unknown;sha256?:unknown;bytes?:unknown}
function supportsVideoDimensions(width:number,height:number):boolean {
 return width>=256&&height>=256&&width<=5760&&height<=5760&&width/height>=0.4&&width/height<=2.5;
}
export function validateInputs(generation:Record<string,unknown>,inputs:Input[],facts:Fact[]):void{
 if(facts.length!==inputs.length)throw Error('Original media facts missing');
 const capability=generation.capability,parameters=(generation.parameters??{}) as Record<string,unknown>;
 if(capability==='speech.transcribe'){
  const input=inputs[0],fact=facts[0],duration=Number(fact?.durationMs);
  if(inputs.length!==1||input?.kind!=='audio'||input.role!=='source_audio'||fact?.sha256!==input.asset.sha256||fact.bytes!==input.asset.bytes||!Number.isSafeInteger(input.asset.bytes)||input.asset.bytes<=0||input.asset.bytes>50000000||!['audio/wav','audio/x-wav','audio/mpeg','audio/flac','audio/aac','audio/ogg','audio/opus','audio/mp4'].includes(String(fact.mime))||!Number.isFinite(duration)||duration<=0||duration>500000||parameters.source_duration_ms!==Math.ceil(duration))throw Error('Fixed ASR source facts do not match');
  return;
 }
 if(generation.model==='MiniMax-H3-Regeneration'){
  if(inputs.length||generation.prompt||typeof parameters.sourceJob!=='string'||!parameters.sourceJob)throw Error('Original draft required');
  return;
 }
 const video=capability==='video.generate',image=capability==='image.generate';
 if(!image&&!video&&inputs.length)throw Error('This MiniMax model does not accept reference audio; select an audio-reference model');
 const model=String(generation.model??''),multimodal=video&&['MiniMax-H3','MiniMax-H3-auto'].includes(model)&&parameters.input_mode==='reference';
 const max=model==='midjourney-8.2'?4:model.startsWith('nano_banana_2')?10:multimodal?12:video?9:16;
 if(inputs.length>max)throw Error('Too many references');
 let audioDuration=0,videoDuration=0;
 for(const [i,input]of inputs.entries()){
  const fact=facts[i];
  if(fact.sha256!==input.asset.sha256||fact.bytes!==input.asset.bytes||!Number.isSafeInteger(input.asset.bytes)||input.asset.bytes<=0)throw Error('Original media facts mismatch');
  if(input.kind==='video'&&multimodal){
   const duration=Number(fact.durationMs),w=Number(fact.width),h=Number(fact.height);
   if(fact.mime!=='video/mp4'||input.asset.bytes>50000000||!Number.isSafeInteger(duration)||duration<2000||duration>15000||
      !Number.isSafeInteger(w)||!Number.isSafeInteger(h)||!supportsVideoDimensions(w,h))throw Error('Original reference video unsupported');
   videoDuration+=duration;
   continue;
  }
  if(input.kind==='audio'&&multimodal){
   const duration=Number(fact.durationMs);
   if(!['audio/wav','audio/x-wav','audio/mpeg'].includes(String(fact.mime))||input.asset.bytes>15*1024*1024||!Number.isFinite(duration)||duration<2000||duration>15000)throw Error('Original audio unsupported');
   audioDuration+=duration;
   continue;
  }
  if(input.kind!=='image'||!['image/jpeg','image/png','image/webp'].includes(String(fact.mime))||input.asset.bytes>30*1024*1024)throw Error('Original image unsupported');
  const w=Number(fact.width),h=Number(fact.height);
  if(!Number.isSafeInteger(w)||!Number.isSafeInteger(h)||w<1||h<1||video&&!supportsVideoDimensions(w,h))throw Error('Original image dimensions unsupported');
 }
 if(video){
  if(multimodal){
   if(!inputs.length||inputs.filter(v=>v.kind==='image').length>9||inputs.filter(v=>v.kind==='video').length>3||
      inputs.filter(v=>v.kind==='audio').length>3||videoDuration>15000||audioDuration>15000)throw Error('Original reference limits exceeded');
   return;
  }
  const counts:Record<string,[number,number]>={text:[0,0],first_frame:[1,1],first_last_frame:[2,2],reference_image:[1,9]};
  const range=counts[String(parameters.input_mode)];
  if(!range||inputs.length<range[0]||inputs.length>range[1])throw Error('Input mode does not match original references');
 }
 if(image&&parameters.size!=='auto'){
  const match=/^(\d+)x(\d+)$/.exec(String(parameters.size));
  const w=Number(match?.[1]),h=Number(match?.[2]);
  if(!match||w*h<655360||w*h>20000000||w/h<1/3||w/h>3)throw Error('Custom image size unsupported');
 }
 if(image&&parameters.disable_prompt_enhanced===true&&parameters.size==='auto'&&parameters.aspect_ratio==='auto')throw Error('Explicit image size or ratio required');
}
