import {useEffect,useState} from 'react';
import {Button,Checkbox,Input,Modal,ModalBody,ModalContent,ModalFooter,ModalHeader,Tooltip} from '@heroui/react';
import {Download,Pause,Play,X} from 'lucide-react';
import {useApp} from './context';
import {dateTime,ErrorNote,scheduleTime} from './components';
import type {Creator} from './types';

export function HistoryModal({creator,onClose}:{creator:Creator|null;onClose:()=>void}){
 const {act}=useApp();
 const [limit,setLimit]=useState('50');const [videos,setVideos]=useState(true);const [dynamics,setDynamics]=useState(true);const [busy,setBusy]=useState(false);const [error,setError]=useState('');
 useEffect(()=>{if(creator){setLimit('50');setVideos(creator.watchVideos);setDynamics(creator.watchDynamics);setError('')}},[creator?.id]);
 const start=async()=>{
  const n=Number(limit);if(!limit.trim()||!Number.isInteger(n)||n<0||n>100000||(!videos&&!dynamics)){setError('请选择内容类型，数量为 0–100000');return}
  setBusy(true);try{if(await act(`/creators/${creator!.id}/history`,'POST',{limit:n,watchVideos:videos,watchDynamics:dynamics},'历史拉取已开始'))onClose()}finally{setBusy(false)}
 };
 return <Modal isOpen={!!creator} onClose={onClose} size="lg" scrollBehavior="inside" classNames={{base:'app-modal history-modal'}}><ModalContent><ModalHeader><div><h2>拉取历史</h2><p className="muted">{creator?.name}</p></div></ModalHeader><ModalBody data-slot="body"><div className="history-types"><Checkbox isSelected={videos} onValueChange={setVideos}>视频</Checkbox><Checkbox isSelected={dynamics} onValueChange={setDynamics}>图文动态</Checkbox></div><Input label="最多新增任务" labelPlacement="outside-top" variant="bordered" type="number" min={0} max={100000} value={limit} onValueChange={setLimit} description="0 表示全部历史"/>{error&&<ErrorNote>{error}</ErrorNote>}</ModalBody><ModalFooter data-slot="footer"><Button variant="light" onPress={onClose}>取消</Button><Button color="primary" startContent={<Download size={16}/>} isLoading={busy} onPress={start}>开始拉取</Button></ModalFooter></ModalContent></Modal>
}

export function HistoryActions({creator}:{creator:Creator}){
 const {act}=useApp();const h=creator.historyJob;if(!h||!['running','paused','failed'].includes(h.status))return null;
 const action=(name:string)=>act(`/creators/${creator.id}/history/${name}`,'POST');
 return <div className="history-actions">{h.status==='running'?<Button size="sm" variant="light" startContent={<Pause size={14}/>} onPress={()=>action('pause')}>暂停拉取</Button>:<Button size="sm" variant="light" startContent={<Play size={14}/>} onPress={()=>action('resume')}>继续拉取</Button>}<Button size="sm" variant="light" startContent={<X size={14}/>} onPress={()=>action('cancel')}>取消拉取</Button></div>;
}

export function HistoryStats({creator,compact=false}:{creator:Creator;compact?:boolean}){
 const h=creator.historyJob;if(!h)return null;
 const details=`已读 ${h.pages} 页 · 入队视频 ${h.videos} · 图文 ${h.dynamics}${h.limit>0?` · 上限 ${h.limit} 条`:' · 全部历史'}`;
 if(compact)return <span title={details} aria-label={details}>历史 {h.pages} 页 · 入队 {h.videos+h.dynamics} 条</span>;
 return <div className="creator-activity-facts"><span>已读 {h.pages} 页</span><span>视频 {h.videos}</span><span>图文 {h.dynamics}</span><span>{h.limit>0?`上限 ${h.limit} 条`:'全部历史'}</span></div>;
}

export function HistoryProgress({creator}:{creator:Creator}){
 const {state}=useApp();const h=creator.historyJob;if(!h)return null;
 const labels={running:'拉取中',paused:'已暂停',completed:'拉取完成',cancelled:'已取消',failed:'拉取失败'};
 const cooling=state.bilibiliBlockedUntil&&new Date(state.bilibiliBlockedUntil)>new Date();
 const waiting=state.paused?'全局调度已暂停':cooling?`冷却至 ${scheduleTime(state.bilibiliBlockedUntil!)}`:new Date(h.nextPageAt)>new Date()?`下页 ${scheduleTime(h.nextPageAt)}`:'等待队列或调度';
 const error=h.error||creator.lastError;
 return <div className="creator-activity-panel history-progress"><div className="creator-activity-header"><div className="creator-activity-heading"><strong>历史 · {labels[h.status]}</strong>{h.status==='running'&&<span className="creator-activity-timing" title={dateTime(h.nextPageAt)}>{waiting}</span>}</div><HistoryActions creator={creator}/></div><div className="creator-activity-details"><HistoryStats creator={creator}/>{error&&<Tooltip content={error} placement="top" classNames={{content:'creator-activity-tooltip'}}><button type="button" className="creator-activity-link" title={error}>查看错误详情</button></Tooltip>}</div></div>;
}
