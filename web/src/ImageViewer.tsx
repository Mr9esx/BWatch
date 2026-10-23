import {Button,Modal,ModalBody,ModalContent,ModalFooter,ModalHeader} from '@heroui/react';
import {ChevronLeft,ChevronRight,ExternalLink} from 'lucide-react';
import type {KeyboardEvent} from 'react';

export default function ImageViewer({sources,index,title,onIndexChange,onClose}:{sources:string[];index:number|null;title:string;onIndexChange:(index:number)=>void;onClose:()=>void}){
 const current=Math.min(Math.max(0,index??0),Math.max(0,sources.length-1));
 const move=(step:number)=>onIndexChange(Math.min(sources.length-1,Math.max(0,current+step)));
 const keyDown=(event:KeyboardEvent)=>{if(event.key==='ArrowLeft'||event.key==='ArrowRight'){event.preventDefault();event.stopPropagation();move(event.key==='ArrowLeft'?-1:1)}};
 return <Modal isOpen={index!==null&&sources.length>0} onClose={onClose} size="full" aria-label="图片大图浏览" classNames={{base:'image-lightbox',wrapper:'image-lightbox-wrapper',backdrop:'image-lightbox-backdrop',closeButton:'image-lightbox-close'}}><ModalContent onKeyDown={keyDown}>
  <ModalHeader className="image-lightbox-header"><h2>{title}</h2><span aria-live="polite">{current+1} / {sources.length}</span></ModalHeader>
  <ModalBody className="image-lightbox-body"><Button isIconOnly variant="flat" aria-label="上一张图片" title="上一张（←）" isDisabled={current===0} onPress={()=>move(-1)}><ChevronLeft size={24}/></Button><div><img src={sources[current]} alt={`${title} · 第 ${current+1} 张图片`} /></div><Button isIconOnly variant="flat" aria-label="下一张图片" title="下一张（→）" isDisabled={current===sources.length-1} onPress={()=>move(1)}><ChevronRight size={24}/></Button></ModalBody>
  <div className="image-lightbox-thumbnails" role="group" aria-label="图片缩略图">{sources.map((source,i)=><button type="button" key={source} className={i===current?'active':''} aria-label={`查看第 ${i+1} 张图片`} aria-current={i===current?'true':undefined} onClick={()=>onIndexChange(i)}><img src={source} alt="" loading="lazy"/></button>)}</div>
  <ModalFooter className="image-lightbox-footer"><span>← → 切换图片 · Esc 关闭</span><a href={sources[current]} target="_blank" rel="noreferrer">查看原图<ExternalLink size={14}/></a></ModalFooter>
 </ModalContent></Modal>
}
