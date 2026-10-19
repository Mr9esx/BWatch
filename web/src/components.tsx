import {Button,Tooltip} from '@heroui/react';
import {ArrowUpRight,Check,Copy,FileText,Film,Inbox,MoreHorizontal,Play,RefreshCw,Search,TriangleAlert,X} from 'lucide-react';
import {useEffect,useRef,useState,type ReactNode} from 'react';
import {useApp} from './context';
import type {Creator,Task} from './types';

export function Logo(){return <span className="brand-mark" aria-hidden="true"><img src="/hamster-logo-bold.png" alt="" width={28} height={28} draggable={false}/></span>}
const colors=['purple','blue','orange','green','pink'];
export function Avatar({name,src,index=0,small=false}:{name:string;src?:string;index?:number;small?:boolean}){
 const [failed,setFailed]=useState(false);useEffect(()=>setFailed(false),[src]);
 return <span className={`creator-avatar ${colors[index%5]} ${small?'small':''}`}>{src&&!failed?<img src={src} alt="" loading="lazy" referrerPolicy="no-referrer" onError={()=>setFailed(true)}/>:name.slice(0,1)}</span>
}
export function CreatorIdentity({creator,index=0}:{creator:Creator;index?:number}){return <div className="identity"><Avatar name={creator.name} src={creator.avatar} index={index}/><div><strong>{creator.name}</strong><span className="muted tiny">{creator.platform==='bilibili'?'bilibili':'YouTube'} <span className="middot">·</span> {creator.url.split('/').at(-2)}</span></div></div>}
export function Status({status}:{status:Task['status']}){const labels={queued:'等待下载',downloading:'下载中',paused:'已暂停',completed:'已完成',failed:'下载失败'};return <span className={`status ${status}`}><i/>{labels[status]}</span>}
export function Kind({kind}:{kind:Task['kind']}){return <span className={`kind ${kind}`}>{kind==='dynamic'?<FileText size={12}/>:<Film size={12}/>} {kind==='dynamic'?'图文动态':'视频'}</span>}
export function Empty({icon=<Inbox size={28}/>,title,description,action}:{icon?:ReactNode;title:string;description?:string;action?:ReactNode}){return <div className="empty"><div className="empty-icon">{icon}</div><h3>{title}</h3>{description&&<p>{description}</p>}{action}</div>}
export function ErrorNote({children}:{children:ReactNode}){return <div className="error-note"><TriangleAlert size={16}/><span>{children}</span></div>}
export function ErrorText({message,className=''}:{message:string;className?:string}){return <Tooltip content={message} delay={0} closeDelay={200} placement="top" classNames={{base:'error-tooltip-layer',content:'error-tooltip'}}><span tabIndex={0} className={`error-detail ${className}`}>{message}</span></Tooltip>}
export function IconButton({label,children,onPress,disabled=false}:{label:string;children:ReactNode;onPress:()=>void;disabled?:boolean}){return <Button isIconOnly size="sm" variant="light" aria-label={label} title={label} onPress={onPress} isDisabled={disabled} className="icon-button">{children}</Button>}
export function FileLocation({task,compact=false}:{task:Task;compact?:boolean}){
 const {notify}=useApp();
 const copy=async()=>{try{await navigator.clipboard.writeText(task.file);notify('保存路径已复制')}catch{notify('无法自动复制，请选中路径文字复制',true)}};
 const path=task.file||'未记录保存路径';
 const code=<code tabIndex={0} aria-label="服务器文件路径">{path}</code>;
 return <div className={`file-location ${compact?'compact-location':''}`}><div><span>服务器文件路径</span>{task.file&&<IconButton label="复制保存路径" onPress={copy}><Copy size={14}/></IconButton>}</div>{compact?<Tooltip content={path} delay={0} closeDelay={200} placement="top" classNames={{content:'path-tooltip'}}>{code}</Tooltip>:code}</div>
}
export function Thumbnail({task,onPress}:{task:Task;onPress?:()=>void}){
 const source=task.status==='completed'?`/api/tasks/${encodeURIComponent(task.id)}/thumbnail`:task.thumbnail;
 return <button className={`thumbnail ${task.kind}`} onClick={onPress} disabled={!onPress} aria-label={`查看 ${task.title}`}>{source?<img key={source} src={source} alt="" loading="lazy" referrerPolicy="no-referrer" onError={e=>{e.currentTarget.style.display='none'}}/>:null}<span className="thumb-fallback">{task.kind==='video'?<Film size={22}/>:<FileText size={22}/>}</span>{task.kind==='video'&&<span className="duration">{duration(task.duration)}</span>}{onPress&&<span className="thumb-play"><Play size={18} fill="currentColor"/></span>}</button>
}
export function duration(seconds:number){return `${Math.floor(seconds/60)}:${String(Math.floor(seconds%60)).padStart(2,'0')}`}
export function bytes(n:number){if(!Number.isFinite(n)||n<=0)return '0 B';const units=['B','KB','MB','GB','TB','PB'];const unit=Math.max(0,Math.min(Math.floor(Math.log(n)/Math.log(1024)),units.length-1));return `${(n/1024**unit).toFixed(unit===0&&n<1?1:unit<2?0:unit===2?1:2)} ${units[unit]}`}
export function relative(date:string|null){if(!date)return '尚未检查';const min=Math.max(0,Math.floor((Date.now()-new Date(date).getTime())/60000));if(min<1)return '刚刚';if(min<60)return `${min} 分钟前`;if(min<1440)return `${Math.floor(min/60)} 小时前`;return `${Math.floor(min/1440)} 天前`}
export function dateTime(date:string){return new Date(date).toLocaleString('zh-CN',{month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',hour12:false})}
export function quality(q:string){return q==='best'?'最佳画质':q==='2160'?'4K':`${q}P`}
export function External({url}:{url:string}){return <a href={url} target="_blank" rel="noreferrer" className="external" aria-label="打开原文" title="打开原文"><ArrowUpRight size={16}/></a>}
export {Check,MoreHorizontal,RefreshCw};

export function PageSearch({placeholder}:{placeholder:string}){
 const {search,setSearch}=useApp();const input=useRef<HTMLInputElement>(null);
 useEffect(()=>{const onKey=(event:KeyboardEvent)=>{if((event.metaKey||event.ctrlKey)&&event.key==='k'){event.preventDefault();input.current?.focus()}};window.addEventListener('keydown',onKey);return()=>window.removeEventListener('keydown',onKey)},[]);
 return <div className="page-search" role="search"><Search size={16}/><input ref={input} aria-label={placeholder} placeholder={placeholder} value={search} onChange={event=>setSearch(event.target.value)}/>{search&&<button type="button" aria-label="清空搜索" onClick={()=>{setSearch('');input.current?.focus()}}><X size={15}/></button>}</div>
}
