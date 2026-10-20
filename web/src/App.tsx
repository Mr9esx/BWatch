import {useCallback,useEffect,useRef,useState} from 'react';
import {Button,Input,Modal,ModalBody,ModalContent,ModalFooter,ModalHeader,Select,SelectItem} from '@heroui/react';
import {ArrowDownToLine,Check,ChevronLeft,ChevronRight,Clapperboard,ExternalLink,FileText,FolderHeart,LayoutDashboard,Menu,Plus,RefreshCw,Settings,Terminal,TriangleAlert,Users,X} from 'lucide-react';
import {request} from './api';
import {AppContext} from './context';
import type {CreatorInput,Health,Page,State,Task} from './types';
import {emptyState} from './types';
import {bytes,Empty,ErrorNote,FileLocation,Kind,Logo,quality,Status} from './components';
import Dashboard from './Dashboard';
import ImageViewer from './ImageViewer';
import BilibiliAccount from './BilibiliAccount';
import {Creators,Library,Tasks} from './ContentPages';
import {Logs,SettingsPage} from './SystemPages';

const nav=[{id:'overview',label:'工作台',icon:LayoutDashboard},{id:'creators',label:'UP 主订阅',icon:Users},{id:'tasks',label:'下载任务',icon:ArrowDownToLine},{id:'library',label:'内容库',icon:FolderHeart},{id:'logs',label:'运行日志',icon:Terminal},{id:'settings',label:'偏好设置',icon:Settings}] as const;
type Confirmation={title:string;description:string;action:()=>Promise<boolean>};
export default function App(){
 const [page,setPage]=useState<Page>(()=>{const hash=location.hash.slice(1);return nav.some(n=>n.id===hash)?hash as Page:'overview'});
 const [state,setState]=useState<State>(emptyState);
 const [imageIndex,setImageIndex]=useState<number|null>(null);
 const [previewImageIndex,setPreviewImageIndex]=useState(0);
 const [health,setHealth]=useState<Health|null>(null);const [connectionError,setConnectionError]=useState('');const [search,setSearch]=useState('');const [addOpen,setAddOpen]=useState(false);const [sidebarOpen,setSidebarOpen]=useState(false);const [selected,setSelected]=useState<Task|null>(null);const [confirmation,setConfirmation]=useState<Confirmation|null>(null);const [confirmBusy,setConfirmBusy]=useState(false);
 useEffect(()=>{setImageIndex(null);setPreviewImageIndex(0)},[selected?.id]);
 const [toast,setToast]=useState<{message:string;error:boolean}|null>(null);const toastTimer=useRef<ReturnType<typeof setTimeout>|null>(null);
 const notify=useCallback((message:string,error=false)=>{setToast({message,error});if(toastTimer.current)clearTimeout(toastTimer.current);toastTimer.current=setTimeout(()=>setToast(null),5000)},[]);
 const refresh=useCallback(async()=>{const [s,h]=await Promise.all([request<State>('/state'),request<Health>('/health')]);setState(s);setHealth(h);setConnectionError('')},[]);
 useEffect(()=>{let active=true;async function poll(){try{const [s,h]=await Promise.all([request<State>('/state'),request<Health>('/health')]);if(active){setState(s);setHealth(h);setConnectionError('')}}catch(e){if(active)setConnectionError(e instanceof Error?e.message:'无法连接服务')}}void poll();const timer=setInterval(poll,3000);return()=>{active=false;clearInterval(timer)}},[]);
 useEffect(()=>{const onHash=()=>{const p=location.hash.slice(1);if(nav.some(n=>n.id===p)){setPage(p as Page);setSearch('')}};window.addEventListener('hashchange',onHash);return()=>window.removeEventListener('hashchange',onHash)},[]);
 useEffect(()=>{const onKey=(e:KeyboardEvent)=>{if(e.key==='Escape'){if(!selected&&!confirmation&&!addOpen)setSearch('');setSidebarOpen(false)}};window.addEventListener('keydown',onKey);return()=>window.removeEventListener('keydown',onKey)},[selected,confirmation,addOpen]);
 const navigate=(p:Page)=>{setPage(p);setSearch('');setSidebarOpen(false);location.hash=p;window.scrollTo({top:0,behavior:'smooth'})};
 const act=async(path:string,method='POST',body?:unknown,message?:string)=>{
  try{
   await request(path,method,body);await refresh();if(message)notify(message);return true;
  }catch(e){notify(e instanceof Error?e.message:'操作失败，请重试',true);return false}
 };
 const active=state.tasks.filter(t=>t.status==='downloading').length;
 const previewTask=selected?(state.tasks.find(t=>t.id===selected.id)||selected):null;
 const previewImages=previewTask?.kind==='dynamic'&&previewTask.status==='completed'?(previewTask.files||[]).map((_,index)=>`/api/tasks/${encodeURIComponent(previewTask.id)}/files/${index+1}`):[];
 return <AppContext.Provider value={{state,health,search,setSearch,page,navigate,openAdd:()=>setAddOpen(true),act,preview:setSelected,notify,confirm:(title,description,action)=>setConfirmation({title,description,action})}}>
  <div className="app-shell">
   {sidebarOpen&&<button className="sidebar-backdrop" aria-label="关闭导航" onClick={()=>setSidebarOpen(false)}/>}
   <aside className={`sidebar ${sidebarOpen?'open':''}`}><a className="brand" href="#overview" onClick={e=>{e.preventDefault();navigate('overview')}}><Logo/><span>Bili<span>Fetch</span></span></a><nav aria-label="主导航">{nav.slice(0,4).map(n=><button key={n.id} onClick={()=>navigate(n.id)} className={`nav-item ${page===n.id?'active':''}`} aria-current={page===n.id?'page':undefined}><n.icon size={19}/><span>{n.label}</span>{n.id==='tasks'&&active>0?<span className="nav-count">{active}</span>:n.id==='creators'&&state.creators.length>0?<span className="nav-count neutral">{state.creators.length}</span>:null}</button>)}{nav.slice(4).map(n=><button key={n.id} onClick={()=>navigate(n.id)} className={`nav-item ${page===n.id?'active':''}`} aria-current={page===n.id?'page':undefined}><n.icon size={19}/><span>{n.label}</span></button>)}</nav><div className="sidebar-bottom"><div className="service-status"><span className={connectionError?'gray-dot':'green-dot'}/><span>{connectionError?'服务未连接':state.paused?'调度已暂停':'服务运行中'}</span></div><BilibiliAccount/></div></aside>
   <div className="main-shell"><header className="mobile-topbar"><button className="mobile-menu" aria-label="打开导航" onClick={()=>setSidebarOpen(true)}><Menu size={20}/></button><span>BiliFetch</span></header>
    <main className="main-content">{connectionError&&<div className="connection-notice"><TriangleAlert size={17}/><span>服务连接失败：{connectionError}</span><button onClick={()=>refresh().catch(()=>notify('仍无法连接，请检查服务是否启动',true))}><RefreshCw size={13}/>重新连接</button></div>}{health&&!health.ytDlp&&page!=='settings'&&<div className="notice-bar"><TriangleAlert size={15}/><span>未安装 yt-dlp，暂时无法下载视频。</span><button onClick={()=>navigate('settings')}>查看运行环境<ChevronRight size={14}/></button></div>}
     {page==='overview'?<Dashboard/>:page==='creators'?<Creators/>:page==='tasks'?<Tasks/>:page==='library'?<Library/>:page==='logs'?<Logs/>:!health?<div className="panel"><Empty title="正在连接服务"/></div>:<SettingsPage/>}
    </main>
   </div>
  </div>
  <AddCreatorModal isOpen={addOpen} onClose={()=>setAddOpen(false)} settings={state.settings} onSubmit={async input=>{const ok=await act('/creators','POST',input,'订阅已添加');if(ok)setAddOpen(false);return ok}}/>
  <Modal isOpen={!!previewTask} onClose={()=>{setImageIndex(null);setSelected(null)}} size="5xl" scrollBehavior="inside" classNames={{base:'app-modal preview-modal',header:'modal-header'}}>
   <ModalContent>
    {previewTask&&<>
     <ModalHeader className="preview-header">
      <div className="preview-heading">
       <h2>{previewTask.title}</h2>
       <div className="preview-meta"><Kind kind={previewTask.kind}/><span>{previewTask.creatorName}</span><Status status={previewTask.status}/></div>
      </div>
     </ModalHeader>
     <ModalBody data-slot="body" className="preview-body">
      <section className="preview-stage" aria-label="内容预览">
       {previewTask.kind==='video'?(previewTask.status==='completed'?<video className="video-player" src={`/api/tasks/${previewTask.id}/files/0`} controls autoPlay={false} preload="metadata"/>:<div className="video-placeholder">{previewTask.thumbnail&&<img src={previewTask.thumbnail} alt="视频封面" referrerPolicy="no-referrer"/>}<div><Clapperboard size={35}/><p>视频下载后可预览</p></div></div>):previewTask.status==='completed'&&previewImages.length===0?<article className="dynamic-preview dynamic-text-only"><p>{previewTask.content||previewTask.title}</p></article>:<article className="dynamic-preview">{previewTask.status==='completed'&&previewImages.length>0?<><div className="dynamic-gallery"><div className="dynamic-main-image"><Button isIconOnly variant="flat" aria-label="上一张图片" isDisabled={previewImageIndex===0} onPress={()=>setPreviewImageIndex(i=>Math.max(0,i-1))}><ChevronLeft size={22}/></Button><button type="button" className="dynamic-main-image-open" aria-label={`放大图片 ${previewImageIndex+1}`} onClick={()=>setImageIndex(previewImageIndex)}><img src={previewImages[previewImageIndex]} alt={`${previewTask.title} · 第 ${previewImageIndex+1} 张图片`} referrerPolicy="no-referrer"/></button><Button isIconOnly variant="flat" aria-label="下一张图片" isDisabled={previewImageIndex===previewImages.length-1} onPress={()=>setPreviewImageIndex(i=>Math.min(previewImages.length-1,i+1))}><ChevronRight size={22}/></Button></div><div className="dynamic-thumbnails" role="list" aria-label="图片缩略图">{previewImages.map((src,i)=><button type="button" role="listitem" className={i===previewImageIndex?'active':''} key={src} aria-label={`查看第 ${i+1} 张图片`} aria-current={i===previewImageIndex?'true':undefined} onClick={()=>setPreviewImageIndex(i)}><img src={src} alt="" loading="lazy" referrerPolicy="no-referrer"/></button>)}</div></div>{previewTask.content&&<p>{previewTask.content}</p>}</>:<>{previewTask.content&&<p>{previewTask.content}</p>}{previewTask.status!=='completed'&&<p className="muted">图片尚未下载。</p>}</>}</article>}
      </section>
      <aside className="preview-details">
       {previewTask.error&&<ErrorNote>{previewTask.error}</ErrorNote>}
       <div className="preview-detail-block"><span className="preview-detail-label">UP 主</span><strong>{previewTask.creatorName}</strong></div>
       <div className="preview-detail-block"><span className="preview-detail-label">内容信息</span><div className="preview-detail-facts"><span>{bytes(previewTask.bytes)}</span>{previewTask.kind==='video'&&<span>{quality(previewTask.quality)}</span>}</div></div>
       {previewTask.status==='completed'&&<details className="preview-file-details"><summary>文件信息与保存路径</summary><FileLocation task={previewTask}/></details>}
      </aside>
     </ModalBody>
     <ModalFooter data-slot="footer" className="preview-footer">
      <Button as="a" href={previewTask.url} target="_blank" rel="noreferrer" variant="bordered" endContent={<ExternalLink size={14}/>}>查看原文</Button>
      {previewTask.status==='completed'&&<Button as="a" href={previewTask.kind==='dynamic'?`/api/tasks/${previewTask.id}/archive`:`/api/tasks/${previewTask.id}/files/0?download=1`} color="primary" startContent={<ArrowDownToLine size={15}/>}>{previewTask.kind==='dynamic'?'下载图文':'下载文件'}</Button>}
     </ModalFooter>
    </>}
   </ModalContent>
  </Modal>
  <ImageViewer sources={previewImages} index={imageIndex} title={previewTask?.title||''} onIndexChange={i=>{setImageIndex(i);setPreviewImageIndex(i)}} onClose={()=>setImageIndex(null)}/>
  <Modal isOpen={!!confirmation} onClose={()=>setConfirmation(null)} size="md" classNames={{base:'app-modal'}}><ModalContent><ModalHeader>{confirmation?.title}</ModalHeader><ModalBody data-slot="body"><p className="muted">{confirmation?.description}</p></ModalBody><ModalFooter data-slot="footer"><Button variant="light" onPress={()=>setConfirmation(null)}>取消</Button><Button color="danger" isLoading={confirmBusy} onPress={async()=>{if(!confirmation)return;setConfirmBusy(true);if(await confirmation.action())setConfirmation(null);setConfirmBusy(false)}}>确认移除</Button></ModalFooter></ModalContent></Modal>
  {toast&&<div role={toast.error?'alert':'status'} className={`toast ${toast.error?'toast-error':''}`}>{toast.error?<TriangleAlert size={19}/>:<Check size={19}/>}<span>{toast.message}</span><button aria-label="关闭通知" onClick={()=>setToast(null)}><X size={15}/></button></div>}
 </AppContext.Provider>
}

function AddCreatorModal({isOpen,onClose,settings,onSubmit}:{isOpen:boolean;onClose:()=>void;settings:State['settings'];onSubmit:(input:CreatorInput)=>Promise<boolean>}){
 const initial=():CreatorInput=>({name:'',url:'',interval:settings.interval,quality:settings.quality,history:false,watchVideos:true,watchDynamics:true});
 const [input,setInput]=useState(initial);const [error,setError]=useState('');const [busy,setBusy]=useState(false);
 useEffect(()=>{if(isOpen){setInput({name:'',url:'',interval:settings.interval,quality:settings.quality,history:false,watchVideos:true,watchDynamics:true});setError('')}},[isOpen,settings.interval,settings.quality]);
 const youtube=input.url.includes('youtube.com');
 const submit=async(e:React.FormEvent)=>{e.preventDefault();setError('');if(!input.watchVideos&&!input.watchDynamics){setError('请至少选择一种监听内容');return};if(input.interval<1||input.interval>1440){setError('检查间隔应为 1–1440 分钟');return};if(!/^\d{1,20}$/.test(input.url.trim())&&!/^https:\/\/(space\.bilibili\.com|(?:www\.)?youtube\.com)\//.test(input.url.trim())){setError('请输入有效的 UID、B 站空间或 YouTube 频道地址');return};setBusy(true);await onSubmit(input);setBusy(false)};
 return <Modal isOpen={isOpen} onClose={onClose} size="xl" scrollBehavior="inside" classNames={{base:'app-modal add-modal',header:'modal-header'}}><ModalContent><form onSubmit={submit}><ModalHeader><div><h2>添加 UP 主订阅</h2></div></ModalHeader><ModalBody data-slot="body"><Input autoFocus isRequired label="UP 主 UID 或主页链接" labelPlacement="outside-top" placeholder="例如 946974 或 https://space.bilibili.com/946974" variant="bordered" value={input.url} onValueChange={v=>{const yt=v.includes('youtube.com');setInput({...input,url:v,watchDynamics:yt?false:input.watchDynamics})}}/><Input label="订阅备注" labelPlacement="outside-top" placeholder="可选，留空自动获取名称" variant="bordered" maxLength={80} value={input.name} onValueChange={v=>setInput({...input,name:v})}/><div className="form-section"><label className="field-label">监听内容</label><div className="content-choices"><label className={input.watchVideos?'selected':''}><input type="checkbox" checked={input.watchVideos} onChange={e=>setInput({...input,watchVideos:e.target.checked})}/><Clapperboard size={20}/><span><strong>视频更新</strong></span><i>{input.watchVideos&&<Check size={12}/>}</i></label><label className={`${input.watchDynamics?'selected':''} ${youtube?'disabled':''}`}><input type="checkbox" checked={input.watchDynamics} disabled={youtube} onChange={e=>setInput({...input,watchDynamics:e.target.checked})}/><FileText size={20}/><span><strong>图文动态</strong>{youtube&&<small>仅支持 B 站</small>}</span><i>{input.watchDynamics&&<Check size={12}/>}</i></label></div></div><div className="form-two-column"><Input label="检查间隔" labelPlacement="outside-top" type="number" min={1} max={1440} isRequired variant="bordered" value={String(input.interval)} onValueChange={v=>setInput({...input,interval:Number(v)})} endContent={<span className="input-unit">分钟</span>}/><Select label="下载画质" labelPlacement="outside-top" variant="bordered" selectedKeys={[input.quality]} onChange={e=>setInput({...input,quality:e.target.value})}><SelectItem key="best">最佳画质</SelectItem><SelectItem key="2160">4K / 2160P</SelectItem><SelectItem key="1080">1080P</SelectItem><SelectItem key="720">720P</SelectItem><SelectItem key="480">480P</SelectItem></Select></div><div className="form-section"><label className="field-label">已有内容</label><div className="history-options"><label className={!input.history?'selected':''}><input type="radio" name="history" checked={!input.history} onChange={()=>setInput({...input,history:false})}/><span><strong>仅监听后续更新</strong></span></label><label className={input.history?'selected':''}><input type="radio" name="history" checked={input.history} onChange={()=>setInput({...input,history:true})}/><span><strong>视频、图文各归档最近 10 条</strong></span></label></div></div>{error&&<ErrorNote>{error}</ErrorNote>}</ModalBody><ModalFooter data-slot="footer"><Button variant="light" onPress={onClose}>取消</Button><Button color="primary" type="submit" isLoading={busy} startContent={!busy&&<Plus size={16}/>}>添加订阅</Button></ModalFooter></form></ModalContent></Modal>
}
