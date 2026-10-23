import {useEffect,useState} from 'react';
import {Button} from '@heroui/react';
import {request} from './api';
import {bytes,duration} from './components';
import type {Task} from './types';
import './media-info.css';

type MediaVideo={codec:string;profile?:string;width:number;height:number;frameRate?:number;pixelFormat?:string;bitRate?:number};
type MediaAudio={codec:string;sampleRate?:number;channels?:number;channelLayout?:string;bitRate?:number};
type MediaMetadata={fileName:string;format:string;size:number;duration?:number;bitRate?:number;video?:MediaVideo;audio:MediaAudio[]};
const positive=(value?:number):value is number=>typeof value==='number'&&Number.isFinite(value)&&value>0;
const rate=(value?:number)=>positive(value)?value>=1e6?`${(value/1e6).toFixed(2)} Mbps`:value>=1000?`${Number((value/1000).toFixed(1))} kbps`:`${value} bps`:undefined;
function Fact({label,value}:{label:string;value?:string|number}){return <div><dt>{label}</dt><dd>{value||'未提供'}</dd></div>}

export function MediaFacts({info,kind}:{info:MediaMetadata;kind:Task['kind']}){
 const image=kind==='image_post';const video=info.video;
 return <>
  <dl className="media-facts">
   <Fact label="文件名" value={info.fileName}/>
   <Fact label="文件大小" value={bytes(info.size)}/>
   <Fact label={image?'图片编码':'视频编码'} value={video?.codec?.toUpperCase()}/>
   <Fact label="分辨率" value={positive(video?.width)&&positive(video?.height)?`${video.width} × ${video.height}`:undefined}/>
   {!image&&<>
    <Fact label="封装格式" value={info.format?.toUpperCase().replaceAll(',',' / ')}/>
    <Fact label="时长" value={positive(info.duration)?duration(info.duration):undefined}/>
    <Fact label="帧率" value={positive(video?.frameRate)?`${Number(video.frameRate.toFixed(2))} fps`:undefined}/>
    <Fact label="总码率" value={rate(info.bitRate)}/>
    <Fact label="视频码率" value={rate(video?.bitRate)}/>
    <Fact label="编码规格" value={video?.profile}/>
   </>}
   <Fact label="像素格式" value={video?.pixelFormat}/>
  </dl>
  {!image&&(info.audio.length?info.audio.map((audio,index)=><div className="media-audio" key={index}><h4>音轨 {index+1}</h4><dl className="media-facts">
   <Fact label="音频编码" value={audio.codec?.toUpperCase()}/>
   <Fact label="音频码率" value={rate(audio.bitRate)}/>
   <Fact label="采样率" value={positive(audio.sampleRate)?`${Number((audio.sampleRate/1000).toFixed(2))} kHz`:undefined}/>
   <Fact label="声道" value={positive(audio.channels)?`${audio.channels} 声道${audio.channelLayout?` · ${audio.channelLayout}`:''}`:audio.channelLayout}/>
  </dl></div>):<p className="muted">音频：无音轨</p>)}
 </>;
}

export default function MediaInfo({taskId,index,kind}:{taskId:string;index:number;kind:Task['kind']}){
 const [result,setResult]=useState<{path:string;info?:MediaMetadata;error?:string}|null>(null);
 const [retry,setRetry]=useState(0);
 const path=`/tasks/${encodeURIComponent(taskId)}/files/${index}/metadata`;
 useEffect(()=>{
  const controller=new AbortController();let active=true;setResult(null);
  request<MediaMetadata>(path,'GET',undefined,controller.signal).then(info=>{if(active)setResult({path,info})}).catch(error=>{if(active)setResult({path,error:error instanceof Error?error.message:'读取媒体信息失败'})});
  return()=>{active=false;controller.abort()};
 },[path,retry]);
 const current=result?.path===path?result:null;
 return <section className="preview-detail-block media-info" aria-label="媒体信息" aria-busy={!current}>
  <h3>媒体信息{kind==='image_post'?` · 第 ${index} 张图片`:''}</h3>
  {!current?<p className="muted" role="status">正在读取媒体信息…</p>:current.info?<MediaFacts info={current.info} kind={kind}/>:<div className="media-info-error"><p role="status">{current.error}</p><Button size="sm" variant="bordered" onPress={()=>setRetry(value=>value+1)}>重新读取</Button></div>}
 </section>;
}
