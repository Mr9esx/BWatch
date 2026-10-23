import {useMemo,useRef,useState} from 'react';
import {Button,Card,CardBody,Chip,Skeleton,Tab,Tabs} from '@heroui/react';
import {ArrowDownToLine,ArrowLeft,ArrowRight,CalendarDays,ChevronLeft,ChevronRight,Database,Film,FolderOpen,HardDrive,Maximize2,Users,X} from 'lucide-react';
import {useApp} from './context';
import {Avatar,bytes,CountBadge,dateTime,Empty,External,FilterButton,FilterSelect,Kind,PageSearch,PlatformBadge,PlatformSelect,quality,relative,Thumbnail,ViewSwitch} from './components';
import type {Creator,Task} from './types';
import {matchesPlatform,platformLabel,type Platform,type PlatformFilter} from './platforms';
import {pageHash,type LibraryView} from './navigation';

type LibraryKind='all'|'video'|'image_post';
type LibrarySort='new'|'old'|'size'|'name'|'published-new'|'published-old';
type LibraryDateRange='all'|'7d'|'30d'|'90d';
type LibraryFilters={platform?:PlatformFilter;creatorId?:string|null;kind?:LibraryKind;query?:string;sort?:LibrarySort;dateRange?:LibraryDateRange};
type ContentFilters={query:string;kind:LibraryKind;dateRange:LibraryDateRange;sort:LibrarySort;layout:'grid'|'list';page:number};
export type LibraryGroup={id:string;platform:Platform;name:string;avatar?:string;tasks:Task[];videos:number;dynamics:number;size:number;latest:Task};
const defaultFilters:ContentFilters={query:'',kind:'all',dateRange:'all',sort:'published-new',layout:'grid',page:1};
const sortOptions:readonly {key:LibrarySort;label:string}[]=[{key:'new',label:'最近归档'},{key:'old',label:'最早归档'},{key:'size',label:'占用最大'},{key:'name',label:'名称排序'}];
const contentSortOptions:readonly {key:LibrarySort;label:string}[]=[{key:'published-new',label:'最新发布'},{key:'published-old',label:'最早发布'},...sortOptions];
const dateRangeOptions:readonly {key:LibraryDateRange;label:string}[]=[{key:'all',label:'全部时间'},{key:'7d',label:'近 7 天'},{key:'30d',label:'近 30 天'},{key:'90d',label:'近 90 天'}];
const archivedAt=(task:Task)=>Date.parse(task.finishedAt||task.createdAt)||0;
const publishedAt=(task:Task)=>task.publishedAt?Date.parse(task.publishedAt):NaN;
function comparePublication(a:Task,b:Task,oldest:boolean){
 const x=publishedAt(a),y=publishedAt(b);
 if(!Number.isFinite(x))return Number.isFinite(y)?1:0;
 if(!Number.isFinite(y))return -1;
 return (oldest?1:-1)*(x-y);
}

export function selectLibraryTasks(tasks:Task[],filters:LibraryFilters={},creators:Creator[]=[],now=Date.now()){
 const {creatorId,platform='all',kind='all',query='',sort='new',dateRange='all'}=filters;
 const names=new Map(creators.map(creator=>[creator.id,creator.name]));
 const needle=query.trim().toLocaleLowerCase();
 const cutoff=dateRange==='all'?null:now-parseInt(dateRange,10)*86400000;
 return tasks.filter(task=>{
  if(task.status!=='completed'||!matchesPlatform(task,platform)||(creatorId&&task.creatorId!==creatorId)||(kind!=='all'&&task.kind!==kind))return false;
  const published=publishedAt(task);
  if(cutoff!==null&&(!Number.isFinite(published)||published<cutoff||published>now))return false;
  return `${names.get(task.creatorId)||''} ${task.creatorName} ${task.title} ${task.content||''} ${task.videoId} ${task.file}`.toLocaleLowerCase().includes(needle);
 }).sort((a,b)=>{
  if(sort==='published-new'||sort==='published-old')return comparePublication(a,b,sort==='published-old')||archivedAt(b)-archivedAt(a);
  if(sort==='size')return b.bytes-a.bytes||archivedAt(b)-archivedAt(a);
  if(sort==='name')return a.title.localeCompare(b.title,'zh-CN');
  return (sort==='old'?1:-1)*(archivedAt(a)-archivedAt(b));
 });
}

export function getLibraryGroups(tasks:Task[],creators:Creator[],sort:LibrarySort='new'){
 const profiles=new Map(creators.map(creator=>[creator.id,creator]));
 const groups=new Map<string,LibraryGroup>();
 for(const task of selectLibraryTasks(tasks)){
  let group=groups.get(task.creatorId);
  if(!group){
   const profile=profiles.get(task.creatorId);
   group={id:task.creatorId,platform:profile?.platform||task.platform,name:profile?.name||task.creatorName||'未命名创作者',avatar:profile?.avatar,tasks:[],videos:0,dynamics:0,size:0,latest:task};
   groups.set(task.creatorId,group);
  }
  group.tasks.push(task);group.size+=task.bytes;
  if(task.kind==='video')group.videos++;else group.dynamics++;
 }
 return [...groups.values()].sort((a,b)=>sort==='size'?b.size-a.size:sort==='name'?a.name.localeCompare(b.name,'zh-CN'):sort==='old'?archivedAt(a.tasks.at(-1)!)-archivedAt(b.tasks.at(-1)!):archivedAt(b.latest)-archivedAt(a.latest));
}

export function LibraryItem({task,layout='grid',showCreator=true}:{task:Task;layout?:'grid'|'list';showCreator?:boolean}){
 const {state,preview,navigate,libraryScope}=useApp();
 const creator=state.creators.find(creator=>creator.id===task.creatorId);
 const name=creator?.name||task.creatorName||'未命名创作者';
 const imageCount=task.files?.length??task.images?.length??0;
 const published=Number.isFinite(publishedAt(task));
 const download=task.kind==='image_post'?`/api/tasks/${encodeURIComponent(task.id)}/archive`:`/api/tasks/${encodeURIComponent(task.id)}/files/0?download=1`;
 const excerpt=(task.content||'').replace(/\s+/g,' ').trim();
 return <article className={`library-item ${layout==='grid'?'library-card panel':'library-row panel'}`}>
  <div className="library-cover">
   {task.kind==='image_post'&&!imageCount?<button type="button" className="library-text-cover" aria-label={`预览文字动态：${task.title}`} onClick={()=>preview(task)}><span>{task.content||task.title}</span></button>:<Thumbnail task={task} onPress={()=>preview(task)}/>}
   <Kind kind={task.kind}/>
   {layout==='grid'&&<PlatformBadge platform={task.platform}/>}
   {task.kind==='image_post'&&<span className="image-count">{imageCount?`${imageCount} 张图片`:'纯文字'}</span>}
  </div>
  <div className="library-card-body">
   {showCreator&&<div className="library-author"><Avatar name={name} src={creator?.avatar} small/><button type="button" className="library-author-link" onClick={()=>navigate('library',{platform:libraryScope.platform,creatorId:task.creatorId,view:'creators'})}>{name}</button>{layout==='list'&&<PlatformBadge platform={task.platform}/>}</div>}
   <button type="button" className="library-title" title={task.title} onClick={()=>preview(task)}>{task.title}</button>
   {excerpt&&excerpt!==task.title&&<p className="library-excerpt">{excerpt}</p>}
   <div className="library-file-meta"><span className="library-publication">{published?<time dateTime={task.publishedAt!}>发布于 {dateTime(task.publishedAt!,true)}</time>:'发布时间待补齐'}</span><span>{task.kind==='video'?quality(task.quality):imageCount?`${imageCount} 张图片`:'纯文字'}</span></div>
   <div className="library-card-footer"><span title={`归档于 ${dateTime(task.finishedAt||task.createdAt,true)}`}>{relative(task.finishedAt||task.createdAt)}归档 · {bytes(task.bytes)}</span><div className="library-item-actions"><Button isIconOnly size="sm" variant="light" aria-label={`查看详情：${task.title}`} title="查看详情" onPress={()=>preview(task)}><Maximize2 size={15}/></Button><Button as="a" size="sm" color="primary" className="library-download" href={download} aria-label={`下载 ${task.title}`} title={task.kind==='image_post'?'下载图文归档包':'下载视频文件'} startContent={<ArrowDownToLine size={15}/>}>下载</Button><External url={task.url}/></div></div>
  </div>
 </article>;
}

export default function Library(){
 const {state,navigate,libraryScope,loading=false,connectionError,reload,notify}=useApp();
 const {creatorId,platform}=libraryScope;
 const view:LibraryView=creatorId?'creators':libraryScope.view||'all';
 const [filtersByScope,setFiltersByScope]=useState<Record<string,ContentFilters>>({});
 const [folderSearch,setFolderSearch]=useState('');
 const [folderSort,setFolderSort]=useState<LibrarySort>('new');
 const resultsRef=useRef<HTMLDivElement>(null);
 const scopeKey=creatorId||'all';
 const filters=filtersByScope[scopeKey]||defaultFilters;
 const updateFilters=(patch:Partial<ContentFilters>)=>setFiltersByScope(previous=>({...previous,[scopeKey]:{...(previous[scopeKey]||defaultFilters),page:1,...patch}}));
 const completed=useMemo(()=>state.tasks.filter(task=>task.status==='completed'),[state.tasks]);
 const groups=useMemo(()=>getLibraryGroups(completed,state.creators),[completed,state.creators]);
 const creator=groups.find(group=>group.id===creatorId)||state.creators.find(profile=>profile.id===creatorId);
 const scoped=useMemo(()=>selectLibraryTasks(completed,{creatorId,platform,query:filters.query,sort:filters.sort},state.creators),[completed,creatorId,platform,filters.query,filters.sort,state.creators]);
 const dated=useMemo(()=>selectLibraryTasks(scoped,{dateRange:filters.dateRange,sort:filters.sort}),[scoped,filters.dateRange,filters.sort]);
 const items=dated.filter(task=>filters.kind==='all'||task.kind===filters.kind);
 const missingDates=scoped.filter(task=>!Number.isFinite(publishedAt(task))).length;
 const toView=(next:LibraryView)=>navigate('library',{platform,view:next});
 const reset=()=>{updateFilters({...defaultFilters,layout:filters.layout});if(platform!=='all'&&!creatorId)navigate('library',{view})};
 const onPage=(page:number)=>{updateFilters({page});resultsRef.current?.scrollIntoView({block:'start',behavior:'smooth'})};
 const retry=()=>void reload?.().catch(error=>notify(error instanceof Error?error.message:'服务仍无法连接',true));
 const unavailable=!!connectionError&&!completed.length;
 const body=loading?<LibraryLoading/>:unavailable?<Empty icon={<Database size={30}/>} title="内容库暂时无法加载" description="连接服务后可重新读取归档内容。" action={<Button color="primary" onPress={retry}>重新加载</Button>}/>:<>
  {creatorId&&<div className="library-creator-scope"><Button size="sm" variant="bordered" onPress={()=>toView('creators')} startContent={<ArrowLeft size={15}/>}>返回创作者列表</Button><Avatar name={creator?.name||'未知创作者'} src={creator?.avatar}/><div><h2>{creator?.name||'未找到该创作者'}</h2><span>{completed.filter(task=>task.creatorId===creatorId).length} 条归档</span></div>{creator&&<PlatformBadge platform={creator.platform}/>}</div>}
  <div className="library-global-toolbar" role="group" aria-label="筛选归档内容"><PageSearch placeholder={creatorId?'搜索该创作者的归档内容':'搜索标题、正文、创作者或 ID'} value={filters.query} onChange={query=>updateFilters({query})}/>{!creatorId&&<PlatformSelect value={platform} onChange={next=>{updateFilters({page:1});navigate('library',{platform:next,view})}}/>}<FilterSelect label="发布时间范围" value={filters.dateRange} onChange={dateRange=>updateFilters({dateRange})} options={dateRangeOptions}/><FilterSelect label="内容排序" value={filters.sort} onChange={sort=>updateFilters({sort})} options={contentSortOptions}/><ViewSwitch value={filters.layout} onChange={layout=>updateFilters({layout,page:filters.page})} label="内容"/></div>
  <div className="library-global-kind-filter" role="group" aria-label="内容类型">{([['all','全部类型'],['video','视频'],['image_post','图文']] as const).map(([key,label])=><FilterButton key={key} selected={filters.kind===key} onPress={()=>updateFilters({kind:key})} count={dated.filter(task=>key==='all'||task.kind===key).length}>{label}</FilterButton>)}</div>
  <div className="library-active-filters"><span role="status" aria-live="polite">找到 <strong>{items.length}</strong> 条内容</span>{filters.query.trim()&&<Chip variant="bordered" radius="sm" onClose={()=>updateFilters({query:''})}>搜索：{filters.query.trim()}</Chip>}{!creatorId&&platform!=='all'&&<Chip variant="bordered" radius="sm" onClose={()=>navigate('library',{view})}>{platformLabel(platform)}</Chip>}{filters.kind!=='all'&&<Chip variant="bordered" radius="sm" onClose={()=>updateFilters({kind:'all'})}>{filters.kind==='video'?'视频':'图文'}</Chip>}{filters.dateRange!=='all'&&<Chip variant="bordered" radius="sm" onClose={()=>updateFilters({dateRange:'all'})}>{dateRangeOptions.find(option=>option.key===filters.dateRange)?.label}</Chip>}{(filters.query.trim()||filters.kind!=='all'||filters.dateRange!=='all'||filters.sort!=='published-new'||(!creatorId&&platform!=='all'))&&<Button size="sm" variant="light" onPress={reset} startContent={<X size={14}/>}>清除筛选</Button>}</div>
  {filters.dateRange!=='all'&&missingDates>0&&<p className="library-date-notice"><CalendarDays size={15}/>{missingDates} 条内容未记录发布时间，未参与本次时间范围筛选。<Button size="sm" variant="light" onPress={()=>updateFilters({dateRange:'all'})}>查看全部时间</Button></p>}
  <div ref={resultsRef} className="library-global-results"><ContentResults items={items} filters={filters} onPage={onPage} showCreator={!creatorId} hasContent={completed.length>0} onReset={reset} onSubscribe={()=>navigate('creators')}/></div>
 </>;
 return <div className="page-enter library-page library-overview">
  <div className="page-heading"><div><h1>内容库</h1><p>集中管理归档，随时查找、浏览和使用。</p></div><Button color="primary" onPress={()=>navigate('creators')} startContent={<Users size={16}/>}>管理订阅</Button></div>
  <LibraryStats tasks={completed} groups={groups} loading={loading}/>
  <section className="library-content-panel panel" aria-label="内容浏览"><span className="library-content-caption">{view==='creators'&&!creatorId?<><FolderOpen size={15}/>按创作者整理</>:<><CalendarDays size={15}/>按发布时间整理</>}</span>
   <Tabs aria-label="内容库视图" variant="light" selectedKey={view} disableCursorAnimation onSelectionChange={key=>toView(key as LibraryView)} classNames={{base:'library-collection-tabs',tabList:'library-collection-tab-list',tab:'library-collection-tab',cursor:'library-collection-tab-cursor',tabContent:'library-collection-tab-content',panel:'library-collection-panel'}}>
    <Tab key="all" title={<span>全部内容 <CountBadge count={completed.length}/></span>}>{body}</Tab>
    <Tab key="creators" title={<span>创作者 <CountBadge count={groups.length}/></span>}>{creatorId?body:loading?<LibraryLoading/>:unavailable?<Empty title="内容库暂时无法加载" action={<Button onPress={retry}>重新加载</Button>}/>:<LibraryFolders groups={groups} platform={platform} onPlatformChange={next=>navigate('library',{platform:next,view:'creators'})} search={folderSearch} setSearch={setFolderSearch} sort={folderSort} onSort={setFolderSort}/>}</Tab>
   </Tabs>
  </section>
 </div>;
}

function LibraryStats({tasks,groups,loading}:{tasks:Task[];groups:LibraryGroup[];loading:boolean}){
 const monthStart=new Date();monthStart.setDate(1);monthStart.setHours(0,0,0,0);
 const videos=tasks.filter(task=>task.kind==='video').length;
 const stats=[
  {label:'已归档内容',value:tasks.length,unit:'条',detail:`本月新增 ${tasks.filter(task=>archivedAt(task)>=monthStart.getTime()).length} 条`,color:'violet',icon:Database},
  {label:'创作者',value:groups.length,unit:'位',detail:`B站 ${groups.filter(group=>group.platform==='bilibili').length} · 小红书 ${groups.filter(group=>group.platform==='xiaohongshu').length}`,color:'mint',icon:Users},
  {label:'视频 / 图文',value:`${videos} / ${tasks.length-videos}`,unit:'',detail:tasks.length?`视频占比 ${Math.round(videos/tasks.length*100)}%`:'暂无归档内容',color:'yellow',icon:Film},
  {label:'归档文件大小',value:bytes(tasks.reduce((sum,task)=>sum+task.bytes,0)),unit:'',detail:'已完成归档文件合计',color:'peach',icon:HardDrive},
 ];
 return <div className="library-stat-grid" aria-label="内容库统计">{stats.map(stat=><section className={`library-stat-card panel ${stat.color}`} key={stat.label}><div className="library-stat-label">{stat.label}<stat.icon size={17}/></div>{loading?<Skeleton className="library-stat-skeleton"/>:<strong>{stat.value}<small>{stat.unit&&` ${stat.unit}`}</small></strong>}<span>{loading?'正在读取归档…':stat.detail}</span></section>)}</div>;
}

function LibraryLoading(){return <div className="library-loading" role="status" aria-label="正在加载内容库"><span>正在加载内容库…</span><div className="library-loading-grid">{[0,1,2].map(index=><div className="library-loading-card" key={index}><Skeleton className="library-loading-cover"/><Skeleton className="library-loading-line"/><Skeleton className="library-loading-line short"/></div>)}</div></div>}

function ContentResults({items,filters,onPage,showCreator,hasContent,onReset,onSubscribe}:{items:Task[];filters:ContentFilters;onPage:(page:number)=>void;showCreator:boolean;hasContent:boolean;onReset:()=>void;onSubscribe:()=>void}){
 const pageSize=24;
 const pageCount=Math.max(1,Math.ceil(items.length/pageSize));
 const page=Math.min(filters.page,pageCount);
 const start=(page-1)*pageSize;
 const visible=items.slice(start,start+pageSize);
 if(!items.length)return <Empty icon={<FolderOpen size={30}/>} title={hasContent?'没有找到符合条件的内容':'暂无归档内容'} description={hasContent?'调整关键词或筛选条件，再试一次。':'创作者内容下载完成后，会自动整理到这里。'} action={hasContent?<Button variant="bordered" onPress={onReset}>清除筛选</Button>:<Button color="primary" onPress={onSubscribe} endContent={<ArrowRight size={15}/>}>添加创作者订阅</Button>}/>;
 return <><div className={filters.layout==='grid'?'library-grid':'library-list'}>{visible.map(task=><LibraryItem key={task.id} task={task} layout={filters.layout} showCreator={showCreator}/>)}</div><div className="library-pagination" aria-label="内容分页"><span>{start+1}–{Math.min(start+pageSize,items.length)} / {items.length} 条内容</span><div><Button isIconOnly size="sm" variant="bordered" isDisabled={page===1} aria-label="上一页" onPress={()=>onPage(page-1)}><ChevronLeft size={15}/></Button><span aria-live="polite">第 {page} / {pageCount} 页</span><Button isIconOnly size="sm" variant="bordered" isDisabled={page===pageCount} aria-label="下一页" onPress={()=>onPage(page+1)}><ChevronRight size={15}/></Button></div></div></>;
}

function LibraryFolders({groups,platform,onPlatformChange,search,setSearch,sort,onSort}:{groups:LibraryGroup[];platform:PlatformFilter;onPlatformChange:(platform:PlatformFilter)=>void;search:string;setSearch:(value:string)=>void;sort:LibrarySort;onSort:(sort:LibrarySort)=>void}){
 const needle=search.trim().toLocaleLowerCase();
 const folders=groups.filter(group=>(platform==='all'||group.platform===platform)&&`${group.name} ${platformLabel(group.platform)}`.toLocaleLowerCase().includes(needle)).sort((a,b)=>sort==='size'?b.size-a.size:sort==='name'?a.name.localeCompare(b.name,'zh-CN'):sort==='old'?archivedAt(a.tasks.at(-1)!)-archivedAt(b.tasks.at(-1)!):archivedAt(b.latest)-archivedAt(a.latest));
 const reset=()=>{setSearch('');onSort('new');onPlatformChange('all')};
 return <div className="library-folder-browser">
  <div className="library-creator-toolbar" role="group" aria-label="创作者文件夹筛选"><PageSearch placeholder="搜索创作者" value={search} onChange={setSearch}/><PlatformSelect value={platform} onChange={onPlatformChange}/><FilterSelect label="创作者排序" value={sort} onChange={onSort} options={sortOptions}/></div>
  <div className="library-active-filters"><span role="status" aria-live="polite">找到 <strong>{folders.length}</strong> 位创作者</span>{(needle||sort!=='new'||platform!=='all')&&<Button size="sm" variant="light" onPress={reset} startContent={<X size={14}/>}>清除创作者筛选</Button>}</div>
  {folders.length?<section className="library-folder-grid" aria-label="创作者归档文件夹">{folders.map((group,index)=><Card as="a" href={pageHash('library',{platform,creatorId:group.id,view:'creators'})} key={group.id} className="panel library-folder-card" aria-label={`打开 ${group.name} 的归档文件夹`}><CardBody className="library-folder-body"><div className="library-folder-card-heading"><Avatar name={group.name} src={group.avatar} index={index}/><div><h2>{group.name}</h2><PlatformBadge platform={group.platform}/></div><span className="library-folder-symbol" aria-hidden="true"><FolderOpen size={22}/></span></div><div className="library-folder-count"><CountBadge count={group.tasks.length}/><span>条归档</span><span className="library-folder-size">{bytes(group.size)}</span></div><div className="library-folder-types"><span>视频 {group.videos}</span><span>图文 {group.dynamics}</span></div></CardBody><div className="library-folder-footer"><span title={dateTime(group.latest.finishedAt||group.latest.createdAt,true)}>最近归档 · {relative(group.latest.finishedAt||group.latest.createdAt)}</span><ChevronRight size={18} aria-hidden="true"/></div></Card>)}</section>:<Empty icon={<Users size={30}/>} title={groups.length?'没有找到符合条件的创作者':'暂无创作者归档'} description={groups.length?'尝试其他名称或清除筛选。':'内容归档完成后，会自动按创作者整理。'} action={groups.length?<Button variant="bordered" onPress={reset}>清除创作者筛选</Button>:undefined}/>}
 </div>;
}
