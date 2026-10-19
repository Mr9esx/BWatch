import {useEffect,useState} from 'react';
import {Activity,ArrowDown,ArrowUp,Cpu,HardDrive,MemoryStick} from 'lucide-react';
import {request} from './api';
import {bytes,ErrorNote} from './components';
import type {SystemMetrics} from './types';

type RatePoint={time:number;receive:number;send:number};
const rate=(value:number|null|undefined)=>value==null?'正在采样…':`${bytes(value)}/s`;

function Meter({value,label}:{value:number;label:string}){
 const bounded=Math.min(100,Math.max(0,value));
 return <div className="resource-meter" role="progressbar" aria-label={label} aria-valuemin={0} aria-valuemax={100} aria-valuenow={bounded}><i style={{width:`${bounded}%`}}/></div>
}

function NetworkTrend({history}:{history:RatePoint[]}){
 if(history.length<2)return <p className="resource-hint">收集趋势数据中…</p>;
 const max=Math.max(1,...history.flatMap(point=>[point.receive,point.send]));
 const start=history[0].time;const duration=Math.max(1,history.at(-1)!.time-start);
 const points=(key:'receive'|'send')=>history.map(point=>`${4+(point.time-start)/duration*292},${60-point[key]/max*52}`).join(' ');
 return <svg className="network-trend" viewBox="0 0 300 68" preserveAspectRatio="none" role="img" aria-label="最近网络接收与发送速率趋势"><path d="M4 60H296"/><polyline className="receive" points={points('receive')}/><polyline className="send" points={points('send')}/></svg>
}

export function ResourceStats({metrics,error='',history=[]}:{metrics:SystemMetrics|null;error?:string;history?:RatePoint[]}){
 const unavailable=metrics||error?'暂不可用':'正在采集…';
 const diskLow=metrics?.disk!=null&&metrics.disk.usagePercent>=90;
 const networkReady=metrics?.network?.receiveRate!=null&&metrics.network.sendRate!=null;
 return <section className="resource-monitor" aria-label="服务器资源">
  <header className="resource-heading"><h2><Activity size={17}/>服务器资源</h2><span className="muted" role="status">{metrics?`每 3 秒刷新 · ${new Date(metrics.collectedAt).toLocaleTimeString('zh-CN',{hour12:false})}`:error?'连接中断':'正在连接监控…'}</span></header>
  {error&&<ErrorNote>{error}</ErrorNote>}
  <div className="resource-grid">
   <article className="panel resource-card cpu-resource"><h3><Cpu size={17}/>CPU 使用率</h3>{metrics?.cpu?<><strong className="resource-value">{metrics.cpu.usagePercent.toFixed(1)}%</strong><Meter value={metrics.cpu.usagePercent} label="CPU 使用率"/><p>{metrics.cpu.cores} 个逻辑核</p></>:<p className="resource-value">{unavailable}</p>}</article>
   <article className="panel resource-card memory-resource"><h3><MemoryStick size={17}/>内存占用</h3>{metrics?.memory?<><strong className="resource-value">{metrics.memory.usagePercent.toFixed(1)}%</strong><Meter value={metrics.memory.usagePercent} label="内存占用"/><p>已用 {bytes(metrics.memory.used)} / {bytes(metrics.memory.total)}</p><p>可用 {bytes(metrics.memory.available)}</p></>:<p className="resource-value">{unavailable}</p>}</article>
   <article className="panel resource-card network-resource"><h3><Activity size={17}/>网络速率</h3>{metrics?.network?<><div className="network-rates"><span><ArrowDown size={15}/>接收 <strong>{rate(metrics.network.receiveRate)}</strong></span><span><ArrowUp size={15}/>发送 <strong>{rate(metrics.network.sendRate)}</strong></span></div>{networkReady?<NetworkTrend history={history}/>:<p className="resource-hint">等待下一次采样计算速率</p>}</>:<p className="resource-value">{unavailable}</p>}</article>
   <article className={`panel resource-card disk-resource ${diskLow?'storage-low':''}`}><h3><HardDrive size={17}/>磁盘占用{diskLow&&<span className="storage-warning">空间紧张</span>}</h3>{metrics?.disk?<><strong className="resource-value">{metrics.disk.usagePercent.toFixed(1)}%</strong><Meter value={metrics.disk.usagePercent} label="磁盘占用"/><p>已用 {bytes(metrics.disk.used)} / {bytes(metrics.disk.total)}</p><p>可用 {bytes(metrics.disk.available)}</p><code className="resource-path">{metrics.disk.path}</code></>:<p className="resource-value">{unavailable}</p>}</article>
  </div>
  {!!metrics?.warnings?.length&&<ErrorNote>{metrics.warnings.join('；')}</ErrorNote>}
  <p className="resource-scope">服务所在环境的系统指标；网速为活动非回环网卡合计，包含其他程序流量。磁盘为下载目录所在文件系统；Docker 部署时按容器可见范围采集。</p>
 </section>
}

export default function ResourceMonitor(){
 const [metrics,setMetrics]=useState<SystemMetrics|null>(null);const [error,setError]=useState('');const [history,setHistory]=useState<RatePoint[]>([]);
 useEffect(()=>{
  let active=true,pending=false,controller:AbortController|undefined;
  const poll=async()=>{
   if(pending)return;pending=true;controller=new AbortController();const current=controller;const timeout=setTimeout(()=>current.abort(),5000);
   try{
    const result=await request<SystemMetrics>('/metrics','GET',undefined,current.signal);if(!active)return;
    setMetrics(result);setError('');
    const receive=result.network?.receiveRate,send=result.network?.sendRate;
    if(receive!=null&&send!=null)setHistory(previous=>{const time=new Date(result.collectedAt).getTime();return previous.at(-1)?.time===time?previous:[...previous,{time,receive,send}].slice(-60)});
   }catch(e){if(active){setMetrics(null);setHistory([]);setError(e instanceof Error&&e.name!=='AbortError'?e.message:'资源采集超时，请检查服务连接')}}
   finally{clearTimeout(timeout);pending=false}
  };
  void poll();const timer=setInterval(poll,3000);return()=>{active=false;clearInterval(timer);controller?.abort()};
 },[]);
 return <ResourceStats metrics={metrics} error={error} history={history}/>
}
