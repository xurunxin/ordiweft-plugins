export interface ChannelResult {id:string;state:'done';route:'api'|'subscription';media:{kind:string;url?:string;data?:string;voice?:Record<string,unknown>}[];metadata?:Record<string,unknown>}
export function channelResult(value:unknown,id:string,kind:string):ChannelResult{
 if(!value||typeof value!=='object')throw Error('Original result missing');
 const v=value as ChannelResult;
 if(v.id!==id||v.state!=='done'||!['api','subscription'].includes(v.route)||!Array.isArray(v.media)||v.media.length<1||v.media.length>10)throw Error('Original result unconfirmed');
 for(const item of v.media){
  if(item.kind!==kind||(item.url===undefined)===(item.data===undefined)||item.url!==undefined&&(kind==='transcript'||!item.url.startsWith('https://'))||item.data!==undefined&&(!['audio','transcript'].includes(kind)||!item.data||item.data.length>(kind==='transcript'?6:56)*1024*1024))throw Error('Original media unconfirmed');
  if(item.voice&&(kind!=='audio'||item.voice.provider!=='minimax'||typeof item.voice.id!=='string'||!item.voice.id))throw Error('Original voice unconfirmed');
 }
 return v;
}
