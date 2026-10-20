import {useEffect,useState} from 'react';
import {Alert,Button,Modal,ModalBody,ModalContent,ModalFooter,ModalHeader,Tooltip} from '@heroui/react';
import {CheckCircle2,Clapperboard,Download,FileText,FolderOpen,Pause,Play,RefreshCw,Search,TriangleAlert,X} from 'lucide-react';
import {useApp} from './context';
import {HistoryActions,HistoryProgress,HistoryStats} from './History';
import {bytes,dateTime,Empty,ErrorText,External,IconButton,Kind,quality,relative,scheduleTime,Status,Thumbnail} from './components';
import type {Creator,Task} from './types';

const statusOrder={downloading:0,queued:1,failed:2,paused:3,completed:4};
const statusLabels={downloading:'下载中',queued:'等待中',completed:'已完成',failed:'失败',paused:'已暂停'};
export function getCreatorTasks(tasks:Task[],creatorId:string){
 return tasks.filter(task=>task.creatorId===creatorId).sort((a,b)=>statusOrder[a.status]-statusOrder[b.status]||new Date(b.createdAt).getTime()-new Date(a.createdAt).getTime());
}

function isBilibiliTask(task:Task){
 try{const host=new URL(task.url).hostname;return host==='bilibili.com'||host.endsWith('.bilibili.com')}catch{return false}
}

export function CreatorContentCounts({creator}:{creator:Creator}){
 const {state}=useApp();
 const pulled={video:new Set<string>(),dynamic:new Set<string>()};
 const catalog=new Set(Object.entries(creator.contentCatalog||{}).filter(([,known])=>known).map(([key])=>key));
 for(const task of state.tasks){
  if(task.creatorId!==creator.id||!task.videoId)continue;
  pulled[task.kind].add(task.videoId);
  if(!creator.contentCatalogComplete)catalog.add(`${task.kind}:${task.videoId}`);
 }
 if(!creator.contentCatalogComplete){
  for(const [kind,seen] of [['video',creator.seen],['dynamic',creator.seenDynamics]] as const){
   for(const [id,known] of Object.entries(seen||{}))if(known&&id)catalog.add(`${kind}:${id}`);
  }
 }
 return <div className="creator-content-counts" title="全部优先采用 B 站空间统计，图文为 Opus 口径；平台总量缺失时 ≥ 表示已发现的数量。已拉取为现存下载任务，包含未完成的任务。">{(['video','dynamic'] as const).filter(kind=>kind==='video'||creator.platform==='bilibili').map(kind=>{
  const label=kind==='video'?'视频':'图文';const remote=kind==='video'?creator.contentTotals?.videos:creator.contentTotals?.dynamics;const validRemote=typeof remote==='number'&&Number.isInteger(remote)&&remote>=0;const complete=validRemote||!!creator.contentCatalogComplete;const total=validRemote?remote:[...catalog].filter(key=>key.startsWith(`${kind}:`)).length;
  const opus=kind==='dynamic'&&validRemote;const totalLabel=opus?'作品总量':'全部';const pulledLabel=opus?'动态已拉取':'已拉取';
  const Icon=kind==='video'?Clapperboard:FileText;
  return <section className={`creator-content-stat ${kind}`} key={kind} title={opus?'已拉取动态含转发，与作品总量口径不同；已拉取含未完成任务':'已拉取含未完成任务'} aria-label={`${label}：${totalLabel}${complete?' ':'至少 '}${total}，${pulledLabel} ${pulled[kind].size}${opus?'（含转发）':''}`}><h3><Icon size={15}/>{label}</h3><dl><div><dt>{totalLabel}</dt><dd>{complete?'':'≥'}{total}</dd></div><div><dt>已拉取</dt><dd>{pulled[kind].size}</dd></div></dl></section>
 })}</div>
}

function DownloadProgress({task,compact=false}:{task:Task;compact?:boolean}){
 const progress=Math.min(100,Math.max(0,task.progress));
 return <div className="creator-download-progress"><div role="progressbar" aria-label={`${task.title} 下载进度`} aria-valuemin={0} aria-valuemax={100} aria-valuenow={progress}><i style={{width:`${progress}%`}}/></div><div><span>{progress.toFixed(1)}%</span><span>{task.speed||'正在连接…'}{!compact&&task.eta&&` · 剩余 ${task.eta}`}</span></div></div>
}

export function CreatorDownloadSummary({creator,onOpenDownloads}:{creator:Creator;onOpenDownloads:()=>void}){
 const {state}=useApp();const tasks=getCreatorTasks(state.tasks,creator.id);
 const active=tasks.filter(task=>task.status==='downloading');const queued=tasks.filter(task=>task.status==='queued').length;const failed=tasks.filter(task=>task.status==='failed').length;const paused=tasks.filter(task=>task.status==='paused').length;
 const current=active[0];
 const historyActive=creator.historyJob&&['running','paused','failed'].includes(creator.historyJob.status);
 const checkError=creator.lastError&&<Tooltip content={creator.lastError} placement="top" classNames={{content:'creator-activity-tooltip'}}><button type="button" className="creator-activity-link" title={creator.lastError}>查看检查错误</button></Tooltip>;
 if(!current&&!queued&&historyActive)return <HistoryProgress creator={creator}/>;
 if(!current&&!queued&&!failed&&!paused)return <Alert role="status" color="success" variant="flat" title="暂无待处理下载" description={!creator.enabled?'恢复监听后继续检查新内容':state.paused?'恢复调度后继续检查新内容':'发现新内容后会自动加入下载列表'} icon={<CheckCircle2 size={18}/>} endContent={checkError} classNames={{base:'creator-activity-panel creator-idle-alert',mainWrapper:'creator-idle-content',title:'creator-idle-title',description:'creator-idle-description',iconWrapper:'creator-idle-icon'}}/>;
 if(!current&&queued){
  // 调度器按原始队列顺序认领任务，弹窗的新旧排序不能用于预测下一条。
  const next=state.tasks.find(task=>task.creatorId===creator.id&&task.status==='queued')!;
  const now=Date.now();
  const bilibiliAt=Math.max(Date.parse(state.bilibiliBlockedUntil||'')||0,Date.parse(state.nextBilibiliDownloadAt||'')||0);
  const bilibili=isBilibiliTask(next);
  const nextAt=bilibili?bilibiliAt:0;
  const earlier=state.tasks.slice(0,state.tasks.indexOf(next)).some(task=>task.status==='queued'&&(bilibili||bilibiliAt<=now||!isBilibiliTask(task)));
  const busy=state.tasks.filter(task=>task.status==='downloading').length>=state.settings.concurrency;
  const waiting=state.paused?'调度已暂停 · 恢复后按序下载'
   :nextAt>now?`最早 ${scheduleTime(new Date(nextAt).toISOString())} 开始${earlier?' · 前序任务优先':busy?' · 等待下载位空闲':''}`
   :earlier?'等待前序任务，按序开始'
   :busy?'当前下载腾出位置后开始':'即将开始下载';
  return <div className="creator-activity-panel creator-download-summary creator-next-download">
   <div className="creator-activity-header">
    <span className="creator-activity-status"><Download size={14}/>下一条{next.kind==='video'?'视频':'图文'}</span>
    <Tooltip content={next.title} placement="top" classNames={{content:'creator-activity-tooltip'}}>
     <button type="button" className="creator-activity-title" title={next.title} aria-label={`查看下一条下载：${next.title}`} onClick={onOpenDownloads}>{next.title}</button>
    </Tooltip>
   </div>
   <div className="creator-activity-details">
    <span className="creator-activity-timing">{waiting}</span>
    {(historyActive||checkError)&&<div className="creator-activity-links">{historyActive&&<HistoryActions creator={creator}/>} {checkError}</div>}
   </div>
  </div>;
 }
 const label=current?`下载中 ${active.length}${queued?` · 等待 ${queued}`:''}`:failed?`失败 ${failed}`:`已暂停 ${paused}`;
 const Icon=current?Download:failed?TriangleAlert:paused?Pause:CheckCircle2;
 const pending=tasks.filter(task=>task.status===(failed?'failed':'paused'));
 return <div className="creator-activity-panel creator-download-summary"><div className="creator-activity-header"><div className="creator-activity-heading"><span className={`creator-activity-status ${current?'is-downloading':failed?'has-failure':''}`}><Icon size={14}/>{label}</span>{current?<Tooltip content={current.title} placement="top" classNames={{content:'creator-activity-tooltip'}}><button type="button" className="creator-activity-link" title={current.title} aria-label={`查看当前下载：${current.title}`} onClick={onOpenDownloads}>查看当前{current.kind==='video'?'视频':'图文'}下载</button></Tooltip>:<span className="creator-activity-timing">{failed?'可重试失败任务':'可继续暂停任务'}</span>}</div>{historyActive&&<HistoryActions creator={creator}/>}</div><div className={`creator-activity-details ${current?'creator-activity-downloading':''}`}>{current?<>{historyActive?<HistoryStats creator={creator} compact/>:<span className="creator-activity-timing">{current.kind==='video'?`视频 · ${quality(current.quality)}`:'图文动态'}</span>}<DownloadProgress task={current} compact/>{checkError}</>:<><div className="creator-activity-facts"><span>视频 {pending.filter(task=>task.kind==='video').length}</span><span>图文 {pending.filter(task=>task.kind==='dynamic').length}</span><span>已完成 {tasks.filter(task=>task.status==='completed').length}</span></div>{checkError}</>}</div></div>;
}

export function CreatorCheckStatus({creator}:{creator:Creator}){
 const {state,health}=useApp();
 const checking=health?.checking.includes(creator.id);
 const next=Math.max(creator.lastChecked?new Date(creator.lastChecked).getTime()+creator.interval*60000:0,creator.nextCheckAt?new Date(creator.nextCheckAt).getTime():0,creator.platform==='bilibili'&&state.bilibiliBlockedUntil?new Date(state.bilibiliBlockedUntil).getTime():0);
 const nextLabel=!creator.enabled?'监听已暂停':state.paused?'调度已暂停':checking?'正在检查…':next>Date.now()?null:'等待调度';
 const nextDate=new Date(next);const nextText=nextDate.toDateString()===new Date().toDateString()?nextDate.toLocaleTimeString('zh-CN',{hour:'2-digit',minute:'2-digit',hour12:false}):dateTime(nextDate.toISOString());
 return <><div className="creator-check-item"><dt title="最近检查">上次检查</dt><dd>{creator.lastChecked?<time dateTime={creator.lastChecked} title={dateTime(creator.lastChecked)}>{relative(creator.lastChecked)}</time>:'尚未检查'}</dd></div><div className="creator-check-item"><dt title="预计下次检查">下次检查</dt><dd>{nextLabel??<time dateTime={nextDate.toISOString()} title={dateTime(nextDate.toISOString())}>{nextText}</time>}</dd></div></>;
}

export function CreatorDownloadsModal({creator,onClose}:{creator:Creator|null;onClose:()=>void}){
 const {state,act,preview}=useApp();const [filter,setFilter]=useState('all');const [search,setSearch]=useState('');const [busy,setBusy]=useState('');
 useEffect(()=>{setFilter('all');setSearch('')},[creator?.id]);
 const tasks=getCreatorTasks(state.tasks,creator?.id||'');
 const list=tasks.filter(task=>(filter==='all'||task.status===filter)&&`${task.title} ${task.videoId}`.toLowerCase().includes(search.toLowerCase()));
 const action=async(task:Task,name:string)=>{if(busy)return;setBusy(task.id);try{await act(`/tasks/${task.id}/${name}`,'POST')}finally{setBusy('')}};
 const openPreview=(task:Task)=>{onClose();preview(task)};
 return <Modal isOpen={!!creator} onClose={onClose} size="4xl" scrollBehavior="inside" classNames={{base:'app-modal creator-downloads-modal'}}><ModalContent><ModalHeader><h2>{creator?.name} · 下载列表</h2></ModalHeader><ModalBody data-slot="body">{creator&&<CreatorContentCounts creator={creator}/>}<div className="creator-download-filters"><div className="page-search" role="search"><Search size={15}/><input aria-label="搜索该 UP 主的下载任务" placeholder="搜索标题或内容 ID" value={search} onChange={event=>setSearch(event.target.value)}/>{search&&<button type="button" aria-label="清空任务搜索" onClick={()=>setSearch('')}><X size={14}/></button>}</div><select className="filter-select" aria-label="筛选该 UP 主的任务状态" value={filter} onChange={event=>setFilter(event.target.value)}><option value="all">全部任务（{tasks.length}）</option>{Object.entries(statusLabels).map(([key,label])=><option key={key} value={key}>{label}（{tasks.filter(task=>task.status===key).length}）</option>)}</select></div>
  {list.length?<div className="creator-download-list">{list.map(task=><div className="creator-download-row" key={task.id}><Thumbnail task={task} onPress={task.status==='completed'?()=>openPreview(task):undefined}/><div className="creator-download-detail"><div className="creator-download-title"><button type="button" className="task-title" disabled={task.status!=='completed'} title={task.title} onClick={()=>openPreview(task)}>{task.title}</button><Status status={task.status}/></div><div className="creator-download-meta"><Kind kind={task.kind}/>{task.historyJobId&&<span className="history-tag">历史</span>}<span>{bytes(task.bytes)}</span><span>{dateTime(task.createdAt)}</span></div>{task.status==='downloading'&&<DownloadProgress task={task}/>} {task.error&&<ErrorText className="creator-download-error" message={task.error}/>}</div><div className="creator-download-actions">{(task.status==='queued'||task.status==='downloading')&&<IconButton label={`暂停 ${task.title}`} disabled={!!busy} onPress={()=>void action(task,'pause')}><Pause size={16}/></IconButton>}{task.status==='paused'&&<IconButton label={`继续 ${task.title}`} disabled={!!busy} onPress={()=>void action(task,'resume')}><Play size={16}/></IconButton>}{task.status==='failed'&&<IconButton label={`重试 ${task.title}`} disabled={!!busy} onPress={()=>void action(task,'retry')}><RefreshCw size={16}/></IconButton>}{task.status==='completed'&&<IconButton label={`预览 ${task.title}`} onPress={()=>openPreview(task)}><FolderOpen size={16}/></IconButton>}<External url={task.url}/></div></div>)}</div>:<Empty icon={<Download size={28}/>} title={tasks.length?'没有符合条件的任务':'暂无下载任务'}/>}</ModalBody><ModalFooter data-slot="footer"><span className="muted">显示 {list.length} / {tasks.length} 条任务</span><Button variant="light" onPress={onClose}>关闭</Button></ModalFooter></ModalContent></Modal>
}
