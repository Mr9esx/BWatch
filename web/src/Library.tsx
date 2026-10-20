import {useEffect,useRef,useState} from 'react';
import {Button} from '@heroui/react';
import {useWindowVirtualizer} from '@tanstack/react-virtual';
import {ArrowDownToLine,ArrowRight,FileText,FolderOpen,Grid2X2,List,Play,Search,Users,X} from 'lucide-react';
import {useApp} from './context';
import {Avatar,bytes,dateTime,Empty,External,FileLocation,Kind,PageSearch,relative,Thumbnail} from './components';
import type {Creator,Task} from './types';

type LibraryKind='all'|'video'|'dynamic';
type LibrarySort='new'|'old'|'size'|'name';
type LibraryFilters={creatorId?:string|null;kind?:LibraryKind;query?:string;sort?:LibrarySort};
export type LibraryGroup={id:string;name:string;avatar?:string;tasks:Task[];videos:number;dynamics:number;size:number;latest:Task};
const archivedAt=(task:Task)=>new Date(task.finishedAt||task.createdAt).getTime();

export function selectLibraryTasks(tasks:Task[],filters:LibraryFilters={},creators:Creator[]=[]){
 const {creatorId,kind='all',query='',sort='new'}=filters;
 const names=new Map(creators.map(creator=>[creator.id,creator.name]));
 const needle=query.trim().toLocaleLowerCase();
 return tasks.filter(task=>task.status==='completed'&&(!creatorId||task.creatorId===creatorId)&&(kind==='all'||task.kind===kind)&&`${names.get(task.creatorId)||''} ${task.creatorName} ${task.title} ${task.content||''} ${task.videoId} ${task.file}`.toLocaleLowerCase().includes(needle)).sort((a,b)=>sort==='size'?b.bytes-a.bytes:sort==='name'?a.title.localeCompare(b.title,'zh-CN'):(sort==='old'?1:-1)*(archivedAt(a)-archivedAt(b)));
}

export function getLibraryGroups(tasks:Task[],creators:Creator[],sort:LibrarySort='new'){
 const profiles=new Map(creators.map(creator=>[creator.id,creator]));const groups=new Map<string,LibraryGroup>();
 for(const task of selectLibraryTasks(tasks)){
  let group=groups.get(task.creatorId);
  if(!group){const profile=profiles.get(task.creatorId);group={id:task.creatorId,name:profile?.name||task.creatorName||'未命名 UP 主',avatar:profile?.avatar,tasks:[],videos:0,dynamics:0,size:0,latest:task};groups.set(task.creatorId,group)}
  group.tasks.push(task);group.size+=task.bytes;if(task.kind==='video')group.videos++;else group.dynamics++;
 }
 return [...groups.values()].sort((a,b)=>sort==='size'?b.size-a.size:sort==='name'?a.name.localeCompare(b.name,'zh-CN'):sort==='old'?archivedAt(a.tasks.at(-1)!)-archivedAt(b.tasks.at(-1)!):archivedAt(b.latest)-archivedAt(a.latest));
}

export function LibraryItem({task,layout='grid',showAuthor=true}:{task:Task;layout?:'grid'|'list';showAuthor?:boolean}){
 const {state,preview}=useApp();
 const creator=state.creators.find(creator=>creator.id===task.creatorId);
 const imageCount=task.files?.length??task.images?.length??0;
 const download=task.kind==='dynamic'?`/api/tasks/${encodeURIComponent(task.id)}/archive`:`/api/tasks/${encodeURIComponent(task.id)}/files/0?download=1`;
 return <article className={`library-item ${layout==='grid'?'library-card panel':'library-row panel'}`}>
  <div className="library-cover">{task.kind==='dynamic'&&!imageCount?<button type="button" className="library-text-cover" aria-label={`预览文字动态：${task.title}`} onClick={()=>preview(task)}><span>{task.content||task.title}</span></button>:<Thumbnail task={task} onPress={()=>preview(task)}/>}<Kind kind={task.kind}/>{task.kind==='dynamic'&&<span className="image-count">{imageCount?`${imageCount} 张图片`:'纯文字'}</span>}</div>
  <div className="library-card-body">
   <button type="button" className="library-title" title={task.title} onClick={()=>preview(task)}>{task.title}</button>
   {showAuthor&&<div className="library-author"><Avatar name={creator?.name||task.creatorName} src={creator?.avatar} small/><span>{creator?.name||task.creatorName}</span></div>}
   <FileLocation task={task} compact/>
   <div className="library-card-footer"><span title={dateTime(task.finishedAt||task.createdAt)}>{relative(task.finishedAt||task.createdAt)}归档 · {bytes(task.bytes)}</span><div className="library-item-actions"><Button size="sm" variant="light" onPress={()=>preview(task)} startContent={task.kind==='video'?<Play size={14}/>:<FileText size={14}/>}>预览</Button><a className="library-download" href={download} aria-label={`下载 ${task.title}`} title={task.kind==='dynamic'?'下载图文归档包':'下载视频文件'}><ArrowDownToLine size={15}/>下载</a><External url={task.url}/></div></div>
  </div>
 </article>
}

export default function Library(){
 const {state,search,setSearch,navigate}=useApp();
 const [creatorId,setCreatorId]=useState<string|null|undefined>(undefined);const [authorSearch,setAuthorSearch]=useState('');
 const [kind,setKind]=useState<LibraryKind>('all');const [sort,setSort]=useState<LibrarySort>('new');const [layout,setLayout]=useState<'grid'|'list'>('grid');const [loadedCount,setLoadedCount]=useState(24);const [resultsWidth,setResultsWidth]=useState(0);
 const resultsRef=useRef<HTMLElement>(null);const listRef=useRef<HTMLDivElement>(null);const sentinelRef=useRef<HTMLDivElement>(null);const loadingMoreRef=useRef(false);const [scrollMargin,setScrollMargin]=useState(0);
 const complete=state.tasks.filter(task=>task.status==='completed');const groups=getLibraryGroups(complete,state.creators);
 const firstCreatorId=groups[0]?.id;
 useEffect(()=>{if(creatorId===undefined&&firstCreatorId)setCreatorId(firstCreatorId)},[creatorId,firstCreatorId]);
 const activeCreatorId=creatorId===undefined||(creatorId!==null&&!groups.some(group=>group.id===creatorId))?groups[0]?.id??null:creatorId;
 const selected=groups.find(group=>group.id===activeCreatorId);const scope=activeCreatorId?selected?.tasks||[]:complete;
 const authors=groups.filter(group=>`${group.name} ${state.creators.find(creator=>creator.id===group.id)?.url||''}`.toLocaleLowerCase().includes(authorSearch.trim().toLocaleLowerCase()));
 const searched=selectLibraryTasks(scope,{query:search},state.creators);const items=selectLibraryTasks(scope,{kind,query:search,sort},state.creators);
 const total=items.length;const columns=layout==='grid'?Math.max(1,Math.floor((resultsWidth+18)/(280+18))):1;const visibleItems=items.slice(0,loadedCount);const rows=Array.from({length:Math.ceil(visibleItems.length/columns)},(_,index)=>visibleItems.slice(index*columns,(index+1)*columns));
 const rowVirtualizer=useWindowVirtualizer({count:rows.length,estimateSize:()=>layout==='grid'?430:145,overscan:3,scrollMargin,gap:layout==='grid'?18:14,measureElement:element=>element.getBoundingClientRect().height,getItemKey:index=>rows[index]?.[0]?.id??index});
 useEffect(()=>{const element=resultsRef.current;const list=listRef.current;if(!element||!list)return;const update=()=>{setResultsWidth(list.clientWidth);setScrollMargin(list.getBoundingClientRect().top+window.scrollY)};const observer=new ResizeObserver(update);observer.observe(element);observer.observe(list);update();window.addEventListener('resize',update);return()=>{observer.disconnect();window.removeEventListener('resize',update)}},[total]);
 useEffect(()=>{setLoadedCount(24);window.scrollTo({top:0});loadingMoreRef.current=false},[activeCreatorId,kind,sort,search,layout]);
 useEffect(()=>{const sentinel=sentinelRef.current;if(!sentinel||loadedCount>=total)return;const observer=new IntersectionObserver(entries=>{if(entries.some(entry=>entry.isIntersecting)&&!loadingMoreRef.current){loadingMoreRef.current=true;setLoadedCount(count=>Math.min(count+24,total))}},{rootMargin:'500px'});observer.observe(sentinel);return()=>observer.disconnect()},[loadedCount,total]);
 useEffect(()=>{loadingMoreRef.current=false},[loadedCount]);
 const chooseCreator=(id:string|null)=>{setCreatorId(id);setSearch('');window.scrollTo({top:0,behavior:'smooth'})};
 const clearFilters=()=>{setSearch('');setKind('all');setSort('new')};
 const totalSize=complete.reduce((sum,task)=>sum+task.bytes,0);
 return <div className="page-enter library-page">
  <div className="page-heading"><h1>内容库</h1><div className="library-total"><Users size={16}/>{groups.length} 位 UP 主<span className="middot">·</span>{complete.length} 条归档<span className="middot">·</span>{bytes(totalSize)}</div></div>
  <div className="library-workspace">
   <aside className="panel library-sidebar" aria-label="UP 主归档列表">
    <div className="library-sidebar-heading"><h2>UP 主归档</h2><span className="muted">{groups.length}</span></div>
    <div className="page-search" role="search"><Search size={15}/><input aria-label="搜索 UP 主或 UID" placeholder="搜索 UP 主或 UID" value={authorSearch} onChange={event=>setAuthorSearch(event.target.value)}/>{authorSearch&&<button type="button" aria-label="清空 UP 搜索" onClick={()=>setAuthorSearch('')}><X size={14}/></button>}</div>
    <nav className="library-author-list" aria-label="选择归档 UP 主">
     <button type="button" className={`library-author-option ${activeCreatorId===null?'active':''}`} aria-pressed={activeCreatorId===null} onClick={()=>chooseCreator(null)}><span className="library-all-icon"><FolderOpen size={18}/></span><span><strong>全部内容</strong><small>{bytes(totalSize)}</small></span><b>{complete.length}</b></button>
     {authors.map(group=><button type="button" className={`library-author-option ${activeCreatorId===group.id?'active':''}`} aria-pressed={activeCreatorId===group.id} aria-label={`查看 ${group.name} 的归档`} key={group.id} onClick={()=>chooseCreator(group.id)}><Avatar name={group.name} src={group.avatar}/><span><strong title={group.name}>{group.name}</strong><small>视频 {group.videos} · 图文 {group.dynamics}</small></span><b>{group.tasks.length}</b></button>)}
     {!authors.length&&<p className="library-author-empty muted">{groups.length?'没有找到相关 UP 主':'归档后自动显示 UP 主'}</p>}
    </nav>
   </aside>
   <section ref={resultsRef} className="library-results" aria-label="归档内容">
    <header className="library-scope-heading">{selected?<Avatar name={selected.name} src={selected.avatar}/>:<span className="library-all-icon"><FolderOpen size={22}/></span>}<div><h2>{selected?.name||'全部内容'}</h2><p>{scope.length} 条归档 · 视频 {scope.filter(task=>task.kind==='video').length} · 图文 {scope.filter(task=>task.kind==='dynamic').length} · {bytes(selected?.size??totalSize)}</p></div></header>
    <div className="filter-toolbar library-browser-toolbar"><PageSearch placeholder={activeCreatorId?'搜索该 UP 主的标题、内容或路径':'搜索 UP 主、标题、内容或路径'}/><select className="filter-select" aria-label="内容排序" value={sort} onChange={event=>setSort(event.target.value as LibrarySort)}><option value="new">最近归档</option><option value="old">最早归档</option><option value="size">占用最大</option><option value="name">名称排序</option></select></div>
    <div className="library-content-toolbar"><div className="tab-group">{([['all','全部'],['video','视频'],['dynamic','图文动态']] as const).map(([key,label])=><button key={key} className={kind===key?'active':''} aria-pressed={kind===key} onClick={()=>setKind(key)}>{label}<span>{searched.filter(task=>key==='all'||task.kind===key).length}</span></button>)}</div><div className="library-view-tools"><span className="muted" role="status">{total} 条内容{search.trim()?'符合搜索':''}</span><div className="view-switch"><button aria-label="内容卡片视图" aria-pressed={layout==='grid'} className={layout==='grid'?'active':''} onClick={()=>setLayout('grid')}><Grid2X2 size={16}/></button><button aria-label="内容列表视图" aria-pressed={layout==='list'} className={layout==='list'?'active':''} onClick={()=>setLayout('list')}><List size={17}/></button></div></div></div>
    {total>0?<div ref={listRef} className="library-scroll-area" aria-label="滚动浏览归档内容"><div className="library-virtual-canvas" style={{height:rowVirtualizer.getTotalSize()}}>{rowVirtualizer.getVirtualItems().map(virtualRow=><div key={virtualRow.key} ref={rowVirtualizer.measureElement} data-index={virtualRow.index} className={`library-virtual-row ${layout==='grid'?'grid-row':'list-row'}`} style={{position:'absolute',top:0,left:0,width:'100%',transform:`translateY(${virtualRow.start-scrollMargin}px)`,gridTemplateColumns:layout==='grid'?`repeat(${columns},minmax(0,1fr))`:undefined}}>{rows[virtualRow.index].map(task=><LibraryItem key={task.id} task={task} layout={layout} showAuthor={activeCreatorId===null}/>)}</div>)}</div>{loadedCount<total&&<div ref={sentinelRef} className="library-load-sentinel" role="status">向下滚动加载更多…</div>}{loadedCount>=total&&total>24&&<div className="library-load-sentinel" role="status">已显示全部 {total} 条内容</div>}</div>:<div className="panel"><Empty icon={<FolderOpen size={30}/>} title={complete.length?'没有找到相关归档':'暂无归档内容'} description={complete.length?'尝试其他关键词或清除类型筛选。':'归档完成后，会自动按 UP 主整理到这里。'} action={complete.length?<Button variant="flat" onPress={clearFilters}>清除筛选</Button>:<Button color="primary" variant="flat" onPress={()=>navigate('creators')} endContent={<ArrowRight size={15}/>}>前往订阅管理</Button>}/></div>}
   </section>
  </div>
 </div>
}
