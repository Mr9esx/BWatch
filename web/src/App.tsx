import {useCallback,useEffect,useRef,useState} from 'react';
import {AnimatePresence,motion,useReducedMotion} from 'framer-motion';
import {Button,Input,Modal,ModalBody,ModalContent,ModalFooter,ModalHeader,Select,SelectItem} from '@heroui/react';
import {ArrowDownToLine,Check,ChevronLeft,ChevronRight,Clapperboard,Copy,ExternalLink,FileText,FolderHeart,LayoutDashboard,Menu,Plus,RefreshCw,Settings,Terminal,TriangleAlert,Users,X,PanelsTopLeft} from 'lucide-react';
import {request} from './api';
import {pageHash,readRoute,type LibraryDestination} from './navigation';
import {AppContext} from './context';
import type {CreatorInput,Health,Page,State,Task} from './types';
import {emptyState} from './types';
import {bytes,dateTime,CountBadge,Empty,ErrorNote,FileLocation,Kind,Logo,PlatformBadge,quality,Status} from './components';
import Dashboard from './Dashboard';
import ImageViewer from './ImageViewer';
import MediaInfo from './MediaInfo';
import PlatformManagement from './PlatformManagement';
import {Creators,Library,Tasks} from './ContentPages';
import {Logs,SettingsPage} from './SystemPages';

const navGroups=[
 {id:'overview',label:'概览',items:[{id:'overview',label:'工作台',icon:LayoutDashboard}]},
 {id:'content',label:'内容管理',items:[{id:'library',label:'内容库',icon:FolderHeart},{id:'creators',label:'创作者订阅',icon:Users},{id:'tasks',label:'下载任务',icon:ArrowDownToLine}]},
 {id:'system',label:'系统管理',items:[{id:'platforms',label:'平台管理',icon:PanelsTopLeft},{id:'logs',label:'运行日志',icon:Terminal},{id:'settings',label:'偏好设置',icon:Settings}]},
] as const;
type Confirmation={title:string;description:string;action:()=>Promise<boolean>};
export default function App(){
 const reducedMotion=useReducedMotion();const [previewLayoutId,setPreviewLayoutId]=useState<string>();
 const [route,setRoute]=useState(()=>readRoute(location.hash));const {page,libraryScope}=route;
 const [state,setState]=useState<State>(emptyState);
 const [imageIndex,setImageIndex]=useState<number|null>(null);
 const [previewImageIndex,setPreviewImageIndex]=useState(0);
 const [previewMediaError,setPreviewMediaError]=useState(false);const [publicationBusy,setPublicationBusy]=useState(false);
 const [health,setHealth]=useState<Health|null>(null);const [connectionError,setConnectionError]=useState('');const [search,setSearch]=useState('');const [addOpen,setAddOpen]=useState(false);const [sidebarOpen,setSidebarOpen]=useState(false);const [selected,setSelected]=useState<Task|null>(null);const [confirmation,setConfirmation]=useState<Confirmation|null>(null);const [confirmBusy,setConfirmBusy]=useState(false);
 const navigationRef=useRef<HTMLElement>(null);const menuButtonRef=useRef<HTMLButtonElement>(null);
 useEffect(()=>{
  if(!sidebarOpen)return;
  const sidebar=navigationRef.current;if(!sidebar)return;
  const controls=()=>[...sidebar.querySelectorAll<HTMLElement>('a[href],button:not(:disabled)')].filter(element=>element.getClientRects().length&&getComputedStyle(element).visibility!=='hidden');
  (sidebar.querySelector<HTMLElement>('[aria-current="page"]')||controls()[0])?.focus();
  const onKey=(event:KeyboardEvent)=>{if(event.key!=='Tab')return;const items=controls();const first=items[0],last=items.at(-1);if(event.shiftKey&&document.activeElement===first){event.preventDefault();last?.focus()}else if(!event.shiftKey&&document.activeElement===last){event.preventDefault();first?.focus()}};
  const viewport=window.matchMedia('(max-width:620px)');const onResize=()=>{if(!viewport.matches)setSidebarOpen(false)};
  sidebar.addEventListener('keydown',onKey);viewport.addEventListener('change',onResize);
  return()=>{sidebar.removeEventListener('keydown',onKey);viewport.removeEventListener('change',onResize);if(viewport.matches)menuButtonRef.current?.focus({preventScroll:true})};
 },[sidebarOpen]);
 useEffect(()=>{setImageIndex(null);setPreviewImageIndex(0)},[selected?.id]);
 useEffect(()=>{setSelected(null);setImageIndex(null)},[page,libraryScope.creatorId,libraryScope.platform,libraryScope.view]);
 useEffect(()=>{setPreviewMediaError(false)},[selected?.id,previewImageIndex]);
 const [toast,setToast]=useState<{message:string;error:boolean}|null>(null);const toastTimer=useRef<ReturnType<typeof setTimeout>|null>(null);
 const notify=useCallback((message:string,error=false)=>{setToast({message,error});if(toastTimer.current)clearTimeout(toastTimer.current);toastTimer.current=setTimeout(()=>setToast(null),5000)},[]);
 const refresh=useCallback(async()=>{const [s,h]=await Promise.all([request<State>('/state'),request<Health>('/health')]);setState(s);setHealth(h);setConnectionError('')},[]);
 useEffect(()=>{let active=true;async function poll(){try{const [s,h]=await Promise.all([request<State>('/state'),request<Health>('/health')]);if(active){setState(s);setHealth(h);setConnectionError('')}}catch(e){if(active)setConnectionError(e instanceof Error?e.message:'无法连接服务')}}void poll();const timer=setInterval(poll,3000);return()=>{active=false;clearInterval(timer)}},[]);
 useEffect(()=>{const onHash=()=>{setRoute(readRoute(location.hash));setSearch('')};window.addEventListener('hashchange',onHash);return()=>window.removeEventListener('hashchange',onHash)},[]);
 useEffect(()=>{const onKey=(e:KeyboardEvent)=>{if(e.key==='Escape'){if(!sidebarOpen&&!selected&&!confirmation&&!addOpen)setSearch('');setSidebarOpen(false)}};window.addEventListener('keydown',onKey);return()=>window.removeEventListener('keydown',onKey)},[sidebarOpen,selected,confirmation,addOpen]);
 const navigate=(p:Page,destination:LibraryDestination={})=>{const hash=pageHash(p,destination);setRoute(readRoute(hash));setSearch('');setSidebarOpen(false);location.hash=hash;if(p!=='library')window.scrollTo({top:0,behavior:'smooth'})};
 const act=async(path:string,method='POST',body?:unknown,message?:string)=>{
  try{
   await request(path,method,body);await refresh();if(message)notify(message);return true;
  }catch(e){notify(e instanceof Error?e.message:'操作失败，请重试',true);return false}
 };
 const active=state.tasks.filter(t=>t.status==='downloading').length;
 const previewTask=selected?(state.tasks.find(t=>t.id===selected.id)||selected):null;
 const previewImages=previewTask?.kind==='image_post'&&previewTask.status==='completed'?(previewTask.files||[]).map((_,index)=>`/api/tasks/${encodeURIComponent(previewTask.id)}/files/${index+1}`):[];
 return <AppContext.Provider value={{state,health,loading:!health&&!connectionError,connectionError,reload:refresh,search,setSearch,page,libraryScope,navigate,openAdd:()=>setAddOpen(true),act,preview:(task,layoutId)=>{setPreviewLayoutId(layoutId);setSelected(task)},notify,confirm:(title,description,action)=>setConfirmation({title,description,action})}}>
  <div className="app-shell">
   {sidebarOpen&&<button className="sidebar-backdrop" aria-label="关闭导航" onClick={()=>setSidebarOpen(false)}/>}
   <aside ref={navigationRef} id="primary-navigation" role={sidebarOpen?'dialog':undefined} aria-modal={sidebarOpen||undefined} aria-label="站点导航" className={`sidebar ${sidebarOpen?'open':''}`}><a className="brand" href="#overview" onClick={e=>{e.preventDefault();navigate('overview')}}><Logo/><span className="brand-wordmark">Hozon</span></a><button type="button" className="sidebar-close" aria-label="关闭导航" onClick={()=>setSidebarOpen(false)}><X size={18}/></button><nav aria-label="主导航">{navGroups.map(group=><section className="nav-group" key={group.id} aria-labelledby={`nav-group-${group.id}`}><h2 id={`nav-group-${group.id}`} className="nav-group-label">{group.label}</h2>{group.items.map(n=><button key={n.id} onClick={()=>navigate(n.id)} className={`nav-item ${page===n.id?'active':''}`} aria-current={page===n.id?'page':undefined}><n.icon size={19}/><span>{n.label}</span>{n.id==='tasks'&&active>0?<CountBadge count={active} label={`${active} 个下载中任务`}/>:n.id==='creators'&&state.creators.length>0?<CountBadge count={state.creators.length} label={`${state.creators.length} 个订阅`}/>:null}</button>)}</section>)}</nav><div className="sidebar-bottom"><div className="service-status"><span className={connectionError?'gray-dot':'green-dot'}/><span>{connectionError?'服务未连接':state.paused?'调度已暂停':'服务运行中'}</span></div></div></aside>
   <div className="main-shell"><header className="mobile-topbar"><button ref={menuButtonRef} type="button" className="mobile-menu" aria-label="打开导航" aria-controls="primary-navigation" aria-expanded={sidebarOpen} onClick={()=>setSidebarOpen(true)}><Menu size={20}/></button><a className="mobile-brand" href="#overview" aria-label="Hozon，返回工作台" onClick={e=>{e.preventDefault();navigate('overview')}}><Logo/><span className="brand-wordmark">Hozon</span></a></header>
    <main className="main-content">{connectionError&&<div className="connection-notice"><TriangleAlert size={17}/><span>服务连接失败：{connectionError}</span><button onClick={()=>refresh().catch(()=>notify('仍无法连接，请检查服务是否启动',true))}><RefreshCw size={13}/>重新连接</button></div>}{health&&!health.ytDlp&&page!=='settings'&&<div className="notice-bar"><TriangleAlert size={15}/><span>未安装 yt-dlp，暂时无法下载视频。</span><button onClick={()=>navigate('settings')}>查看运行环境<ChevronRight size={14}/></button></div>}
     {page==='overview'?<Dashboard/>:page==='creators'?<Creators/>:page==='platforms'?<PlatformManagement/>:page==='tasks'?<Tasks/>:page==='library'?<Library/>:page==='logs'?<Logs/>:!health?<div className="panel"><Empty title="正在连接服务"/></div>:<SettingsPage/>}
    </main>
   </div>
  </div>
  <AddCreatorModal isOpen={addOpen} onClose={()=>setAddOpen(false)} onManage={()=>{setAddOpen(false);navigate('platforms')}} settings={state.settings} onSubmit={async input=>{await request('/creators','POST',input);setAddOpen(false);notify('订阅已添加');void refresh().catch(()=>{})}}/>
  <Modal isOpen={!!previewTask} onClose={()=>{setImageIndex(null);setSelected(null)}} disableAnimation={!!reducedMotion} motionProps={page==='library'?{initial:{x:40,opacity:0},animate:{x:0,opacity:1},exit:{x:40,opacity:0},transition:{duration:.2}}:previewLayoutId?{initial:false,animate:{opacity:1},exit:{opacity:0},transition:{duration:.34}}:undefined} size="5xl" scrollBehavior="inside" classNames={{base:`app-modal preview-modal ${page==='library'?'library-preview-modal':''}`,wrapper:page==='library'?'library-preview-wrapper':undefined,header:'modal-header'}}>
   <ModalContent as={motion.div} {...{layoutId:reducedMotion?undefined:previewLayoutId,layoutRoot:true,transition:{layout:{duration:.38,ease:[.22,1,.36,1]}},style:{borderRadius:6}}}>
    {previewTask&&<>
     <ModalHeader className="preview-header">
      <div className="preview-heading">
       <h2>{previewTask.title}</h2>
       <div className="preview-meta"><PlatformBadge platform={previewTask.platform}/><Kind kind={previewTask.kind}/><span>{previewTask.creatorName}</span><Status status={previewTask.status}/></div>
      </div>
     </ModalHeader>
     <ModalBody data-slot="body" className="preview-body">
      <section className="preview-stage" aria-label="内容预览">
       {previewMediaError&&<ErrorNote>媒体文件暂时无法读取，可尝试其他图片或查看原文。</ErrorNote>}
       {previewTask.kind==='video'?(previewTask.status==='completed'?<><video className="video-player" src={`/api/tasks/${previewTask.id}/files/0`} controls autoPlay={false} preload="metadata" onError={()=>setPreviewMediaError(true)}/>{previewTask.content&&<article className="dynamic-preview video-description"><p>{previewTask.content}</p></article>}</>:<div className="video-placeholder">{previewTask.thumbnail&&<img src={previewTask.thumbnail} alt="视频封面" referrerPolicy="no-referrer"/>}<div><Clapperboard size={35}/><p>视频下载后可预览</p></div></div>):previewTask.status==='completed'&&previewImages.length===0?<article className="dynamic-preview dynamic-text-only"><p>{previewTask.content||previewTask.title}</p></article>:<article className="dynamic-preview">{previewTask.status==='completed'&&previewImages.length>0?<><div className="dynamic-gallery"><div className="dynamic-main-image"><Button isIconOnly variant="flat" aria-label="上一张图片" isDisabled={previewImageIndex===0} onPress={()=>setPreviewImageIndex(i=>Math.max(0,i-1))}><ChevronLeft size={22}/></Button><button type="button" className="dynamic-main-image-open" aria-label={`放大图片 ${previewImageIndex+1}`} onClick={()=>setImageIndex(previewImageIndex)}><img src={previewImages[previewImageIndex]} alt={`${previewTask.title} · 第 ${previewImageIndex+1} 张图片`} referrerPolicy="no-referrer" onError={()=>setPreviewMediaError(true)}/></button><Button isIconOnly variant="flat" aria-label="下一张图片" isDisabled={previewImageIndex===previewImages.length-1} onPress={()=>setPreviewImageIndex(i=>Math.min(previewImages.length-1,i+1))}><ChevronRight size={22}/></Button></div><div className="dynamic-thumbnails" role="group" aria-label="图片缩略图">{previewImages.map((src,i)=><button type="button" className={i===previewImageIndex?'active':''} key={src} aria-label={`查看第 ${i+1} 张图片`} aria-current={i===previewImageIndex?'true':undefined} onClick={()=>setPreviewImageIndex(i)}><img src={src} alt="" loading="lazy" referrerPolicy="no-referrer"/></button>)}</div></div>{previewTask.content&&<p>{previewTask.content}</p>}</>:<>{previewTask.content&&<p>{previewTask.content}</p>}{previewTask.status!=='completed'&&<p className="muted">图片尚未下载。</p>}</>}</article>}
      </section>
      <aside className="preview-details">
       {previewTask.error&&<ErrorNote>{previewTask.error}</ErrorNote>}
       <div className="preview-detail-block"><span className="preview-detail-label">创作者</span><strong>{state.creators.find(creator=>creator.id===previewTask.creatorId)?.name||previewTask.creatorName}</strong></div>
       <div className="preview-detail-block"><span className="preview-detail-label">发布时间</span>{previewTask.publishedAt&&Number.isFinite(Date.parse(previewTask.publishedAt))?<time dateTime={previewTask.publishedAt}>{dateTime(previewTask.publishedAt,true)}</time>:<><span className="muted">暂未获取</span><Button size="sm" variant="bordered" isLoading={publicationBusy} onPress={async()=>{setPublicationBusy(true);try{await act(`/tasks/${previewTask.id}/publication`,'POST',undefined,'发布时间已补齐')}finally{setPublicationBusy(false)}}}>补齐发布时间</Button></>}</div>
       {previewTask.status==='completed'&&<div className="preview-detail-block"><span className="preview-detail-label">归档时间</span><time dateTime={previewTask.finishedAt||previewTask.createdAt}>{dateTime(previewTask.finishedAt||previewTask.createdAt,true)}</time></div>}
       <div className="preview-detail-block"><span className="preview-detail-label">内容信息</span><div className="preview-detail-facts"><span>{bytes(previewTask.bytes)}</span>{previewTask.kind==='video'&&<span>{quality(previewTask.quality)}</span>}</div></div>
       {previewTask.status==='completed'&&(previewTask.kind==='video'||previewImages.length>0)&&<MediaInfo taskId={previewTask.id} index={previewTask.kind==='video'?0:previewImageIndex+1} kind={previewTask.kind}/>}
       {previewTask.status==='completed'&&<details className="preview-file-details"><summary>文件信息与保存路径</summary><FileLocation task={previewTask}/></details>}
      </aside>
     </ModalBody>
     <ModalFooter data-slot="footer" className="preview-footer">
      {page==='library'&&<Button variant="bordered" startContent={<Copy size={14}/>} onPress={async()=>{try{await navigator.clipboard.writeText(previewTask.url);notify('原文链接已复制')}catch{notify('无法复制链接，请从查看原文打开',true)}}}>复制链接</Button>}
      <Button as="a" href={previewTask.url} target="_blank" rel="noreferrer" variant="bordered" endContent={<ExternalLink size={14}/>}>查看原文</Button>
      {previewTask.status==='completed'&&<Button as="a" href={previewTask.kind==='image_post'?`/api/tasks/${previewTask.id}/archive`:`/api/tasks/${previewTask.id}/files/0?download=1`} color="primary" startContent={<ArrowDownToLine size={15}/>}>{previewTask.kind==='image_post'?'下载图文':'下载文件'}</Button>}
     </ModalFooter>
    </>}
   </ModalContent>
  </Modal>
  <ImageViewer sources={previewImages} index={imageIndex} title={previewTask?.title||''} onIndexChange={i=>{setImageIndex(i);setPreviewImageIndex(i)}} onClose={()=>setImageIndex(null)}/>
  <Modal isOpen={!!confirmation} onClose={()=>setConfirmation(null)} size="md" classNames={{base:'app-modal'}}><ModalContent><ModalHeader>{confirmation?.title}</ModalHeader><ModalBody data-slot="body"><p className="muted">{confirmation?.description}</p></ModalBody><ModalFooter data-slot="footer"><Button variant="light" onPress={()=>setConfirmation(null)}>取消</Button><Button color="danger" isLoading={confirmBusy} onPress={async()=>{if(!confirmation)return;setConfirmBusy(true);if(await confirmation.action())setConfirmation(null);setConfirmBusy(false)}}>确认移除</Button></ModalFooter></ModalContent></Modal>
  <AnimatePresence>{toast&&<motion.div key="toast" initial={{opacity:0,y:reducedMotion?0:16}} animate={{opacity:1,y:0}} exit={{opacity:0,y:reducedMotion?0:8}} transition={{duration:reducedMotion?0:.2}} role={toast.error?'alert':'status'} className={`toast ${toast.error?'toast-error':''}`}>{toast.error?<TriangleAlert size={19}/>:<Check size={19}/>}<span>{toast.message}</span><button aria-label="关闭通知" onClick={()=>setToast(null)}><X size={15}/></button></motion.div>}</AnimatePresence>
 </AppContext.Provider>
}

function AddCreatorModal({isOpen,onClose,settings,onSubmit,onManage}:{isOpen:boolean;onClose:()=>void;settings:State['settings'];onSubmit:(input:CreatorInput)=>Promise<void>;onManage:()=>void}){
 const initial=():CreatorInput=>({platform:'bilibili',name:'',url:'',interval:settings.interval,quality:settings.quality,history:false,watchVideos:true,watchDynamics:true});
 const [input,setInput]=useState(initial);const [platform,setPlatform]=useState<'bilibili'|'xiaohongshu'>('bilibili');const [account,setAccount]=useState<'loading'|'logged_in'|'logged_out'|'expired'|'unavailable'>('loading');const [accountMessage,setAccountMessage]=useState('');const [error,setError]=useState('');const [busy,setBusy]=useState(false);
 const accountCheckId=useRef(0);
 const checkAccount=async(p=platform)=>{const checkId=++accountCheckId.current;setAccount('loading');setAccountMessage('');try{const result=await request<{state:string;message?:string}>(`/platforms/${p}/account`);if(checkId===accountCheckId.current){setAccount(result.state==='logged_in'?'logged_in':result.state==='expired'?'expired':result.state==='unavailable'?'unavailable':'logged_out');setAccountMessage(result.message||'')}}catch{if(checkId===accountCheckId.current){setAccount('unavailable');setAccountMessage('服务未连接')}}};
 useEffect(()=>{if(isOpen){setInput({platform:'bilibili',name:'',url:'',interval:settings.interval,quality:settings.quality,history:false,watchVideos:true,watchDynamics:true});setPlatform('bilibili');setError('')}},[isOpen,settings.interval,settings.quality]);
 useEffect(()=>{if(!isOpen)return;void checkAccount(platform);return()=>{accountCheckId.current++}},[isOpen,platform]);
 const linkValid=platform==='bilibili'?(/^\d{1,20}$/.test(input.url.trim())||/^https:\/\/(?:space\.bilibili\.com\/\d{1,20}(?:[/?#]|$))/.test(input.url.trim())):(/^(?:小红书号[:：]\s*)?[A-Za-z0-9_-]{2,30}$/.test(input.url.trim())||/^https:\/\/(?:(?:www\.)?xiaohongshu\.com\/user\/profile\/[0-9a-fA-F]{24}(?:[/?#]|$)|(?:www\.)?xhslink\.cn\/o\/[^/?#]+)/.test(input.url.trim()));
 const submit=async(e:React.FormEvent)=>{e.preventDefault();setError('');if(account!=='logged_in'){setError('请先登录所选平台账号，再添加订阅');return};if(!linkValid){setError(platform==='bilibili'?'请输入有效的 Bilibili UID 或空间主页地址':'请输入小红书号、App 分享短链或创作者主页链接');return};if(!input.watchVideos&&!input.watchDynamics){setError('请至少选择一种内容类型');return};if(input.interval<1||input.interval>1440){setError('检查间隔应为 1–1440 分钟');return};setBusy(true);try{await onSubmit({...input,platform})}catch(e){setError(e instanceof Error?e.message:'添加订阅失败')}finally{setBusy(false)}};
 const accountLabel=account==='logged_in'?(accountMessage||'已保存登录凭据'):account==='loading'?'正在读取登录凭据…':account==='expired'?'登录已失效':account==='unavailable'?accountMessage||'暂时无法验证登录':'未登录';
 return <Modal isOpen={isOpen} onClose={onClose} size="xl" scrollBehavior="inside" classNames={{base:'app-modal add-modal',header:'modal-header'}}><ModalContent><form onSubmit={submit}><ModalHeader><div><h2>添加创作者订阅</h2></div></ModalHeader><ModalBody data-slot="body"><Select label="内容平台" labelPlacement="outside-top" variant="bordered" selectedKeys={[platform]} onChange={e=>setPlatform(e.target.value as 'bilibili'|'xiaohongshu')}><SelectItem key="bilibili">Bilibili</SelectItem><SelectItem key="xiaohongshu">小红书</SelectItem></Select><div className={`platform-login-hint ${account==='logged_in'?'is-ready':''}`}><span><i/>{accountLabel}</span>{account!=='logged_in'&&<Button size="sm" variant="flat" onPress={onManage}>前往平台管理</Button>}</div>{error&&<ErrorNote>{error}</ErrorNote>}<Input autoFocus isRequired label={platform==='bilibili'?'创作者主页链接':'小红书号或主页链接'} labelPlacement="outside-top" placeholder={platform==='bilibili'?'输入 UID 或 Bilibili 空间主页链接':'输入小红书号、App 分享短链或创作者主页链接'} variant="bordered" value={input.url} onValueChange={v=>setInput({...input,url:v})}/><Input label="订阅备注" labelPlacement="outside-top" placeholder="可选，留空自动获取名称" variant="bordered" maxLength={80} value={input.name} onValueChange={v=>setInput({...input,name:v})}/><div className="form-section"><label className="field-label">订阅内容</label><div className="content-choices"><label className={input.watchVideos?'selected':''}><input type="checkbox" checked={input.watchVideos} onChange={e=>setInput({...input,watchVideos:e.target.checked})}/><Clapperboard size={20}/><span><strong>视频</strong></span><i>{input.watchVideos&&<Check size={12}/>}</i></label><label className={input.watchDynamics?'selected':''}><input type="checkbox" checked={input.watchDynamics} onChange={e=>setInput({...input,watchDynamics:e.target.checked})}/><FileText size={20}/><span><strong>图文</strong></span><i>{input.watchDynamics&&<Check size={12}/>}</i></label></div></div><div className="form-two-column"><Input label="检查间隔" labelPlacement="outside-top" type="number" min={1} max={1440} isRequired variant="bordered" value={String(input.interval)} onValueChange={v=>setInput({...input,interval:Number(v)})} endContent={<span className="input-unit">分钟</span>}/>{input.watchVideos&&<Select label="视频画质" labelPlacement="outside-top" variant="bordered" selectedKeys={[input.quality]} onChange={e=>setInput({...input,quality:e.target.value})}><SelectItem key="best">最佳画质</SelectItem><SelectItem key="2160">4K / 2160P</SelectItem><SelectItem key="1080">1080P</SelectItem><SelectItem key="720">720P</SelectItem><SelectItem key="480">480P</SelectItem></Select>}</div><div className="form-section"><label className="field-label">历史内容</label><div className="history-options"><label className={!input.history?'selected':''}><input type="radio" name="history" checked={!input.history} onChange={()=>setInput({...input,history:false})}/><span><strong>仅监听后续更新</strong></span></label><label className={input.history?'selected':''}><input type="radio" name="history" checked={input.history} onChange={()=>setInput({...input,history:true})}/><span><strong>视频、图文各归档最近 10 条</strong></span></label></div></div></ModalBody><ModalFooter data-slot="footer"><Button variant="light" onPress={onClose}>取消</Button><Button color="primary" type="submit" isDisabled={busy||account!=='logged_in'||!linkValid} isLoading={busy} startContent={!busy&&<Plus size={16}/>}>添加订阅</Button></ModalFooter></form></ModalContent></Modal>
}
