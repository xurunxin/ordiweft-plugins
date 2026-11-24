export type ChannelResult={id:string;state:'done';route:'cli'|'api';media:{kind:string;url:string}[];draft:unknown;metadata?:Record<string,unknown>};
export function channelResult(value:unknown,originalID:string,kind:string):ChannelResult{
 if(!value||typeof value!=='object')throw Error('Original result missing');
 const v=value as ChannelResult;
 if(v.id!==originalID||v.state!=='done'||!['cli','api'].includes(v.route)||!Array.isArray(v.media)||v.media.length<1||v.media.length>100||v.media.some(x=>!x||x.kind!==kind||typeof x.url!=='string'||!x.url.startsWith('https://')))throw Error('Original channel result unconfirmed');
 return v;
}
