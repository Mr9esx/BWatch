import {useEffect,useState} from 'react';
import {Button,Modal,ModalBody,ModalContent,ModalFooter,ModalHeader} from '@heroui/react';
import {QRCodeSVG} from 'qrcode.react';
import {CheckCircle2,QrCode,RefreshCw,Smartphone} from 'lucide-react';
import {request} from './api';
import {useApp} from './context';
import {ErrorNote} from './components';

type Login={id:string;url?:string;expiresAt:string;state:'waiting_scan'|'waiting_confirm'|'success'|'expired';message:string;cookiesFile?:string};
export default function BilibiliLogin({onLogin,compact=false,label}:{onLogin?:(path:string)=>void;compact?:boolean;label?:string}){
 const {notify,state}=useApp();const [open,setOpen]=useState(()=>!compact&&new URLSearchParams(location.search).get('login')==='1');const [login,setLogin]=useState<Login|null>(null);const [busy,setBusy]=useState(false);const [error,setError]=useState('');
 const generate=async()=>{setBusy(true);setError('');setLogin(null);try{setLogin(await request<Login>('/bilibili/login','POST'))}catch(e){setError(e instanceof Error?e.message:'无法申请二维码')}finally{setBusy(false)}};
 useEffect(()=>{if(open)void generate()},[open]);
 useEffect(()=>{
  if(!open||!login||login.state==='success'||login.state==='expired')return;
  let cancelled=false,pending=false;const session=login;
  const timer=setInterval(async()=>{if(pending)return;pending=true;try{const result=await request<Login>(`/bilibili/login/${session.id}/poll`,'POST');if(cancelled)return;setLogin(result);setError('');if(result.state==='success'&&result.cookiesFile){onLogin?.(result.cookiesFile);window.dispatchEvent(new CustomEvent('bilifetch-account-changed',{detail:result.cookiesFile}));notify('B 站登录成功')}}catch(e){if(!cancelled)setError(e instanceof Error?e.message:'扫码状态查询失败')}finally{pending=false}},2200);
  return()=>{cancelled=true;clearInterval(timer)};
 },[open,login?.id,login?.state]);
 return <>{compact?<Button className="account-login" size="sm" variant="light" startContent={<QrCode size={15}/>} onPress={()=>setOpen(true)}>{label||'扫码登录'}</Button>:<div className="bili-login-card"><span className="bili-login-icon"><QrCode size={24}/></span><div><strong>B 站扫码登录</strong>{state.settings.cookiesFile&&<p>已配置登录凭据</p>}</div><Button color="primary" variant="flat" startContent={<QrCode size={15}/>} onPress={()=>setOpen(true)}>{state.settings.cookiesFile?'重新登录':'扫码登录'}</Button></div>}
 <Modal isOpen={open} onClose={()=>setOpen(false)} size="md" classNames={{base:'app-modal'}}><ModalContent><ModalHeader><div><h2>使用哔哩哔哩 App 扫码</h2></div></ModalHeader><ModalBody data-slot="body"><div className="qr-login-area">{busy?<div className="qr-placeholder"><RefreshCw className="animate-spin" size={30}/><p>正在向 B 站申请二维码…</p></div>:login?.state==='success'?<div className="qr-success"><CheckCircle2 size={66}/><h3>登录成功</h3></div>:login?.url?<><div className={`qr-code ${login.state==='expired'?'expired':''}`}><QRCodeSVG value={login.url} size={208} level="M" marginSize={4} title="B 站官方扫码登录二维码"/>{login.state==='expired'&&<div><p>二维码已过期</p><Button size="sm" color="primary" onPress={generate}>刷新二维码</Button></div>}</div><p className="qr-login-status"><Smartphone size={16}/>{login.message}</p></>:<div className="qr-placeholder"><QrCode size={48}/><p>二维码尚未生成</p></div>}</div>{error&&<ErrorNote>{error}</ErrorNote>}</ModalBody><ModalFooter data-slot="footer">{login?.state!=='success'&&<Button variant="light" startContent={<RefreshCw size={14}/>} isLoading={busy} onPress={generate}>重新生成</Button>}<Button color="primary" onPress={()=>setOpen(false)}>{login?.state==='success'?'完成':'关闭'}</Button></ModalFooter></ModalContent></Modal></>
}
