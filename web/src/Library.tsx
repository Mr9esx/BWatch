import {useEffect,useRef,useState} from 'react';
import {BreadcrumbItem,Breadcrumbs,Button,Card,CardBody} from '@heroui/react';
import {useWindowVirtualizer} from '@tanstack/react-virtual';
import {ArrowDownToLine,ArrowRight,ChevronRight,FolderOpen,Users,X} from 'lucide-react';
import {useApp} from './context';
import {Avatar,bytes,CountBadge,dateTime,Empty,External,FileLocation,FilterButton,Kind,PageSearch,PlatformBadge,PlatformSelect,quality,relative,Thumbnail,ViewSwitch} from './components';
import type {Creator,Task} from './types';
import {matchesPlatform,platformLabel,type Platform,type PlatformFilter} from './platforms';
import {pageHash} from './navigation';

type LibraryKind='all'|'video'|'image_post';
type LibrarySort='new'|'old'|'size'|'name';
type LibraryFilters={platform?:PlatformFilter;creatorId?:string|null;kind?:LibraryKind;query?:string;sort?:LibrarySort};
export type LibraryGroup={id:string;platform:Platform;name:string;avatar?:string;tasks:Task[];videos:number;dynamics:number;size:number;latest:Task};
const archivedAt=(task:Task)=>new Date(task.finishedAt||task.createdAt).getTime();

export function selectLibraryTasks(tasks:Task[],filters:LibraryFilters={},creators:Creator[]=[]){
 const {creatorId,platform='all',kind='all',query='',sort='new'}=filters;
 const names=new Map(creators.map(creator=>[creator.id,creator.name]));
 const needle=query.trim().toLocaleLowerCase();
 return tasks.filter(task=>task.status==='completed'&&matchesPlatform(task,platform)&&(!creatorId||task.creatorId===creatorId)&&(kind==='all'||task.kind===kind)&&`${names.get(task.creatorId)||''} ${task.creatorName} ${task.title} ${task.content||''} ${task.videoId} ${task.file}`.toLocaleLowerCase().includes(needle)).sort((a,b)=>sort==='size'?b.bytes-a.bytes:sort==='name'?a.title.localeCompare(b.title,'zh-CN'):(sort==='old'?1:-1)*(archivedAt(a)-archivedAt(b)));
}

export function getLibraryGroups(tasks:Task[],creators:Creator[],sort:LibrarySort='new'){
 const profiles=new Map(creators.map(creator=>[creator.id,creator]));const groups=new Map<string,LibraryGroup>();
 for(const task of selectLibraryTasks(tasks)){
  let group=groups.get(task.creatorId);
  if(!group){const profile=profiles.get(task.creatorId);group={id:task.creatorId,platform:profile?.platform||task.platform,name:profile?.name||task.creatorName||'未命名创作者',avatar:profile?.avatar,tasks:[],videos:0,dynamics:0,size:0,latest:task};groups.set(task.creatorId,group)}
  group.tasks.push(task);group.size+=task.bytes;if(task.kind==='video')group.videos++;else group.dynamics++;
 }
 return [...groups.values()].sort((a,b)=>sort==='size'?b.size-a.size:sort==='name'?a.name.localeCompare(b.name,'zh-CN'):sort==='old'?archivedAt(a.tasks.at(-1)!)-archivedAt(b.tasks.at(-1)!):archivedAt(b.latest)-archivedAt(a.latest));
}

export function LibraryItem({task,layout='grid',showCreator=true}:{task:Task;layout?:'grid'|'list';showCreator?:boolean}){
 const {state,preview}=useApp();
 const creator=state.creators.find(creator=>creator.id===task.creatorId);
 const imageCount=task.files?.length??task.images?.length??0;
 const download=task.kind==='image_post'?`/api/tasks/${encodeURIComponent(task.id)}/archive`:`/api/tasks/${encodeURIComponent(task.id)}/files/0?download=1`;
 return <article className={`library-item ${layout==='grid'?'library-card panel':'library-row panel'}`}>
  <div className="library-cover">{task.kind==='image_post'&&!imageCount?<button type="button" className="library-text-cover" aria-label={`预览文字动态：${task.title}`} onClick={()=>preview(task)}><span>{task.content||task.title}</span></button>:<Thumbnail task={task} onPress={()=>preview(task)}/>} {layout==='grid'&&<><Kind kind={task.kind}/>{task.kind==='image_post'&&<span className="image-count">{imageCount?`${imageCount} 张图片`:'纯文字'}</span>}</>}</div>
  <div className="library-card-body">
   <button type="button" className="library-title" title={task.title} onClick={()=>preview(task)}>{task.title}</button>
   {layout==='list'&&<div className="library-file-meta"><Kind kind={task.kind}/><span>{task.kind==='image_post'?(imageCount?`${imageCount} 张图片`:'纯文字'):quality(task.quality)}</span></div>}
   {showCreator&&<div className="library-author"><Avatar name={creator?.name||task.creatorName} src={creator?.avatar} small/><span>{creator?.name||task.creatorName}</span><PlatformBadge platform={task.platform}/></div>}
   <FileLocation task={task} compact/>
   <div className="library-card-footer"><span title={dateTime(task.finishedAt||task.createdAt)}>{relative(task.finishedAt||task.createdAt)}归档 · {bytes(task.bytes)}</span><div className="library-item-actions"><Button as="a" size="sm" color="primary" className="library-download" href={download} aria-label={`下载 ${task.title}`} title={task.kind==='image_post'?'下载图文归档包':'下载视频文件'} startContent={<ArrowDownToLine size={15}/>}>下载</Button><External url={task.url}/></div></div>
  </div>
 </article>
}

export default function Library(){
 const {libraryScope}=useApp();const [folderSort,setFolderSort]=useState<LibrarySort>('new');
 return libraryScope.creatorId?<CreatorLibrary key={`${libraryScope.platform}:${libraryScope.creatorId}`} creatorId={libraryScope.creatorId} platform={libraryScope.platform}/>:<LibraryFolders sort={folderSort} onSort={setFolderSort}/>;
}

function LibraryFolders({sort,onSort}:{sort:LibrarySort;onSort:(sort:LibrarySort)=>void}){
 const {state,search,setSearch,navigate,libraryScope}=useApp();const {platform}=libraryScope;
 const complete=selectLibraryTasks(state.tasks,{platform});const groups=getLibraryGroups(complete,state.creators,sort);
 const needle=search.trim().toLocaleLowerCase();
 const folders=groups.filter(group=>`${group.name} ${platformLabel(group.platform)} ${state.creators.find(creator=>creator.id===group.id)?.url||''}`.toLocaleLowerCase().includes(needle));
 const filtered=!!needle||platform!=='all'||sort!=='new';
 const reset=()=>{setSearch('');onSort('new');if(platform!=='all')navigate('library')};
 return <div className="page-enter library-page library-folders-page">
  <div className="page-heading"><h1>内容库</h1><div className="library-total"><Users size={16}/>{groups.length} 位创作者<span className="middot">·</span>{complete.length} 条归档<span className="middot">·</span>{bytes(complete.reduce((sum,task)=>sum+task.bytes,0))}</div></div>
  <div className="filter-toolbar library-folder-toolbar" role="group" aria-label="创作者文件夹筛选"><PageSearch placeholder="搜索创作者"/><div className="toolbar-right"><PlatformSelect value={platform} onChange={next=>navigate('library',{platform:next})} label="筛选内容库平台"/><select className="filter-select" aria-label="创作者排序" value={sort} onChange={event=>onSort(event.target.value as LibrarySort)}><option value="new">最近归档</option><option value="old">最早归档</option><option value="size">占用最大</option><option value="name">名称排序</option></select></div></div>
  {filtered&&folders.length>0&&<div className="library-results-summary"><span role="status">{folders.length} 位创作者符合条件</span><Button size="sm" variant="light" startContent={<X size={14}/>} onPress={reset}>重置筛选</Button></div>}
  {folders.length?<section className="library-folder-grid" aria-label="创作者归档文件夹">{folders.map((group,index)=><Card as="a" isPressable href={pageHash('library',{platform,creatorId:group.id})} key={group.id} className="panel library-folder-card" aria-label={`打开 ${group.name} 的归档文件夹`}><CardBody className="library-folder-body"><div className="library-folder-card-heading"><Avatar name={group.name} src={group.avatar} index={index}/><div><h2>{group.name}</h2><PlatformBadge platform={group.platform}/></div><span className="library-folder-symbol" aria-hidden="true"><FolderOpen size={22}/></span></div><div className="library-folder-count"><CountBadge count={group.tasks.length}/><span>条归档</span><span className="library-folder-size">{bytes(group.size)}</span></div><div className="library-folder-types"><span>视频 {group.videos}</span><span>图文 {group.dynamics}</span></div></CardBody><div className="library-folder-footer"><span title={dateTime(group.latest.finishedAt||group.latest.createdAt)}>最近归档 · {relative(group.latest.finishedAt||group.latest.createdAt)}</span><ChevronRight size={18} aria-hidden="true"/></div></Card>)}</section>:<div className="panel"><Empty icon={<FolderOpen size={30}/>} title={filtered?'没有找到符合条件的创作者':'暂无归档内容'} description={filtered?'尝试其他关键词或重置筛选。':'内容归档完成后，会按创作者整理成文件夹。'} action={filtered?<Button variant="flat" onPress={reset}>重置筛选</Button>:<Button color="primary" onPress={()=>navigate('creators')} endContent={<ArrowRight size={15}/>}>前往订阅管理</Button>}/></div>}
 </div>;
}

function CreatorLibrary({creatorId,platform}:{creatorId:string;platform:PlatformFilter}){
 const {state,search,setSearch}=useApp();
 const [kind,setKind]=useState<LibraryKind>('all');const [sort,setSort]=useState<LibrarySort>('new');const [layout,setLayout]=useState<'grid'|'list'>('grid');const [loadedCount,setLoadedCount]=useState(24);const [resultsWidth,setResultsWidth]=useState(0);
 const resultsRef=useRef<HTMLElement>(null);const listRef=useRef<HTMLDivElement>(null);const sentinelRef=useRef<HTMLDivElement>(null);const loadingMoreRef=useRef(false);const [scrollMargin,setScrollMargin]=useState(0);
 const scope=selectLibraryTasks(state.tasks,{platform,creatorId});const group=getLibraryGroups(scope,state.creators)[0];const profile=state.creators.find(creator=>creator.id===creatorId);
 const selected=group||profile;const name=selected?.name||'该创作者';const parent=pageHash('library',{platform});
 const searched=selectLibraryTasks(scope,{query:search},state.creators);const items=selectLibraryTasks(scope,{kind,query:search,sort},state.creators);
 const total=items.length;const columns=layout==='grid'?Math.max(1,Math.floor((resultsWidth+18)/(280+18))):1;const visibleItems=items.slice(0,loadedCount);const rows=Array.from({length:Math.ceil(visibleItems.length/columns)},(_,index)=>visibleItems.slice(index*columns,(index+1)*columns));
 const rowVirtualizer=useWindowVirtualizer({count:rows.length,estimateSize:()=>layout==='grid'?370:165,overscan:3,scrollMargin,gap:layout==='grid'?18:14,measureElement:element=>element.getBoundingClientRect().height,getItemKey:index=>rows[index]?.[0]?.id??index});
 useEffect(()=>{const element=resultsRef.current;const list=listRef.current;if(!element||!list)return;const update=()=>{setResultsWidth(list.clientWidth);setScrollMargin(list.getBoundingClientRect().top+window.scrollY)};const observer=new ResizeObserver(update);observer.observe(element);observer.observe(list);update();window.addEventListener('resize',update);return()=>{observer.disconnect();window.removeEventListener('resize',update)}},[total,layout]);
 useEffect(()=>{setLoadedCount(24);window.scrollTo({top:0});loadingMoreRef.current=false},[kind,sort,search,layout]);
 useEffect(()=>{const sentinel=sentinelRef.current;if(!sentinel||loadedCount>=total)return;const observer=new IntersectionObserver(entries=>{if(entries.some(entry=>entry.isIntersecting)&&!loadingMoreRef.current){loadingMoreRef.current=true;setLoadedCount(count=>Math.min(count+24,total))}},{rootMargin:'500px'});observer.observe(sentinel);return()=>observer.disconnect()},[loadedCount,total]);
 useEffect(()=>{loadingMoreRef.current=false},[loadedCount]);
 const filtered=kind!=='all'||!!search.trim()||sort!=='new';
 const reset=()=>{setSearch('');setKind('all');setSort('new')};
 return <div className="page-enter library-page library-folder-view">
  <Breadcrumbs aria-label="内容库路径" className="library-breadcrumb"><BreadcrumbItem href={parent}>内容库</BreadcrumbItem><BreadcrumbItem isCurrent>{name}</BreadcrumbItem></Breadcrumbs>
  <div className="page-heading"><div className="library-folder-heading"><Avatar name={name} src={selected?.avatar}/><div><div className="library-folder-name"><h1>{name}</h1>{selected&&<PlatformBadge platform={selected.platform}/>}</div><p>{scope.length} 条归档 · {bytes(scope.reduce((sum,task)=>sum+task.bytes,0))}</p></div></div></div>
  <section ref={resultsRef} className="library-results" aria-label="创作者归档内容">
   <div className="filter-toolbar library-browser-toolbar" role="group" aria-label="内容筛选"><div className="tab-group" role="group" aria-label="内容类型">{([['all','全部类型'],['video','视频'],['image_post','图文']] as const).map(([key,label])=><FilterButton key={key} selected={kind===key} onPress={()=>setKind(key)} count={searched.filter(task=>key==='all'||task.kind===key).length}>{label}</FilterButton>)}</div><div className="toolbar-right"><PageSearch placeholder="搜索该创作者的归档内容"/><select className="filter-select" aria-label="内容排序" value={sort} onChange={event=>setSort(event.target.value as LibrarySort)}><option value="new">最近归档</option><option value="old">最早归档</option><option value="size">占用最大</option><option value="name">名称排序</option></select><ViewSwitch value={layout} onChange={setLayout} label="内容"/></div></div>
   {filtered&&total>0&&<div className="library-results-summary"><span role="status">{total} 条内容符合条件</span><Button size="sm" variant="light" onPress={reset} startContent={<X size={14}/>}>清除内容筛选</Button></div>}
   {total>0?<div ref={listRef} className="library-scroll-area" aria-label="滚动浏览归档内容"><div className="library-virtual-canvas" style={{height:rowVirtualizer.getTotalSize()}}>{rowVirtualizer.getVirtualItems().map(virtualRow=><div key={virtualRow.key} ref={rowVirtualizer.measureElement} data-index={virtualRow.index} className={`library-virtual-row ${layout==='grid'?'grid-row':'list-row'}`} style={{position:'absolute',top:0,left:0,width:'100%',transform:`translateY(${virtualRow.start-scrollMargin}px)`,gridTemplateColumns:layout==='grid'?`repeat(${columns},minmax(0,1fr))`:undefined}}>{rows[virtualRow.index].map(task=><LibraryItem key={task.id} task={task} layout={layout} showCreator={false}/>)}</div>)}</div>{loadedCount<total&&<div ref={sentinelRef} className="library-load-sentinel" role="status">向下滚动加载更多…</div>}{loadedCount>=total&&total>24&&<div className="library-load-sentinel" role="status">已显示全部 {total} 条内容</div>}</div>:<div className="panel"><Empty icon={<FolderOpen size={30}/>} title={scope.length?'没有找到相关归档':selected?'该创作者暂无归档内容':'未找到该创作者的归档'} description={scope.length?'尝试其他关键词或清除内容筛选。':selected?'已完成的归档会显示在这个文件夹中。':'可以返回内容库选择其他创作者。'} action={scope.length?<Button variant="flat" onPress={reset}>清除内容筛选</Button>:<Button as="a" href={parent} variant="flat">返回创作者文件夹</Button>}/></div>}
  </section>
 </div>;
}
