import {Button} from '@heroui/react';
import {ArrowRight,ChevronRight,Download,Plus} from 'lucide-react';
import {useApp} from './context';
import {bytes,CountBadge,Empty,Kind,relative,Status,Thumbnail} from './components';
import ResourceMonitor from './ResourceMonitor';

export default function Dashboard(){
 const {state,navigate,openAdd,preview}=useApp();
 const completed=state.tasks.filter(t=>t.status==='completed');
 const active=state.tasks.filter(t=>t.status==='downloading'||t.status==='queued');
 const totalBytes=completed.reduce((n,t)=>n+t.bytes,0);
 const dates=Array.from({length:7},(_,i)=>{const d=new Date();d.setDate(d.getDate()-6+i);d.setHours(0,0,0,0);return d});
 const daily=dates.map(d=>{const tasks=completed.filter(t=>t.finishedAt&&new Date(t.finishedAt).toDateString()===d.toDateString());return{video:tasks.filter(t=>t.kind==='video').length,dynamic:tasks.filter(t=>t.kind==='image_post').length}});
 const max=Math.max(4,...daily.map(d=>d.video+d.dynamic));
 const stats=[{label:'订阅创作者',value:state.creators.length,unit:'位',sub:`${state.creators.filter(c=>c.enabled).length} 位正在监听`},{label:'已归档内容',value:completed.length,unit:'条',sub:`视频 ${completed.filter(t=>t.kind==='video').length} · 图文 ${completed.filter(t=>t.kind==='image_post').length}`},{label:'下载任务',value:active.length,unit:'个',sub:`${state.tasks.filter(t=>t.status==='downloading').length} 下载中 · ${state.tasks.filter(t=>t.status==='queued').length} 等待中`},{label:'已归档大小',value:bytes(totalBytes).split(' ')[0],unit:bytes(totalBytes).split(' ')[1],sub:''}];
 return <div className="page-enter">
  <div className="page-heading"><h1>工作台</h1><Button color="primary" onPress={openAdd} startContent={<Plus size={16}/>}>添加订阅</Button></div>
  <div className="stats-grid">{stats.map(s=><section className="stat-card panel" key={s.label}><div className="stat-top"><span>{s.label}</span></div><div className="stat-number">{s.value}<span>{s.unit}</span></div>{s.sub&&<div className="stat-sub">{s.sub}</div>}</section>)}</div>
  <ResourceMonitor/>
  <div className="dashboard-middle"><section className="panel activity-panel"><div className="panel-heading"><div><h2>归档趋势</h2></div><div className="chart-legend"><span><i/>视频</span><span><i/>图文</span><span className="period-chip">近 7 天</span></div></div><div className="chart"><div className="chart-axis">{[max,Math.round(max*.75),Math.round(max*.5),Math.round(max*.25),0].map((n,i)=><span key={i}>{n}</span>)}</div><div className="chart-body"><div className="chart-grid">{[0,1,2,3,4].map(n=><i key={n}/>)}</div><div className="chart-columns">{daily.map((d,i)=><div className="chart-column" key={i}><div className="bars" title={`${dates[i].toLocaleDateString()}：视频 ${d.video}，图文 ${d.dynamic}`}><div className="bar video" style={{height:`${d.video/max*100}%`}}/><div className="bar dynamic" style={{height:`${d.dynamic/max*100}%`}}/></div><span className={i===6?'today':''}>{i===6?'今天':`${dates[i].getMonth()+1}/${dates[i].getDate()}`}</span></div>)}</div></div></div><div className="chart-footer"><span><span className="trend-dot"/> 本周已归档 <strong>{daily.reduce((n,d)=>n+d.video+d.dynamic,0)}</strong> 条内容</span><button onClick={()=>navigate('library')}>查看内容库<ArrowRight size={13}/></button></div></section>

  </div>
  <div className="dashboard-bottom"><section className="panel recent-tasks"><div className="panel-heading"><div><h2>最近任务 <CountBadge count={state.tasks.length}/></h2></div><button className="text-link" onClick={()=>navigate('tasks')}>全部任务<ChevronRight size={15}/></button></div>{state.tasks.length?<div className="mini-task-list">{[...state.tasks].reverse().sort((a,b)=>Number(b.status==='downloading')-Number(a.status==='downloading')).slice(0,3).map(t=><div className="mini-task" key={t.id}><Thumbnail task={t} onPress={()=>preview(t)}/><div className="mini-task-info"><strong>{t.title}</strong><span>{t.creatorName}<span className="middot">·</span><Kind kind={t.kind}/></span></div><div className="mini-task-status"><Status status={t.status}/>{t.status==='downloading'&&<span className="tiny muted">{t.progress.toFixed(1)}% · {t.speed}</span>}</div></div>)}</div>:<Empty icon={<Download size={25}/>} title="暂无任务" action={<button className="text-link" onClick={openAdd}>添加订阅<ArrowRight size={14}/></button>}/>}</section>
   <section className="panel recent-events"><div className="panel-heading"><div><h2>最近日志</h2></div><button className="text-link" onClick={()=>navigate('logs')} aria-label="查看全部日志">全部日志<ChevronRight size={15}/></button></div>{state.events.length?<div className="event-timeline">{[...state.events].reverse().slice(0,4).map(e=><div className={`timeline-item ${e.level}`} key={e.id}><i/><div><p>{e.message}</p><span>{relative(e.time)}</span></div></div>)}</div>:<Empty title="暂无日志"/>}</section>
  </div>

 </div>
}
