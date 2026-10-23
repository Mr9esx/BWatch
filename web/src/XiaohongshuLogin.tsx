import {useEffect,useRef,useState} from 'react';
import {Button,Input,Modal,ModalBody,ModalContent,ModalFooter,ModalHeader} from '@heroui/react';
import {CheckCircle2,RefreshCw,Smartphone} from 'lucide-react';
import {QRCodeSVG} from 'qrcode.react';
import {request} from './api';
import {ErrorNote} from './components';

type Login={id:string;url?:string;expiresAt:string;state:'waiting_scan'|'waiting_confirm'|'waiting_code'|'success'|'expired';message:string};

export default function XiaohongshuLogin({onLogin}:{onLogin:()=>void}){
 const [open,setOpen]=useState(false);
 const [method,setMethod]=useState<'qr'|'sms'>('qr');
 const [login,setLogin]=useState<Login|null>(null);
 const [phone,setPhone]=useState('');
 const [code,setCode]=useState('');
 const [busy,setBusy]=useState(false);
 const [error,setError]=useState('');
 const [pollFailed,setPollFailed]=useState(false);
 const [retryAt,setRetryAt]=useState(0);
 const [now,setNow]=useState(Date.now());
 const current=useRef<Login|null>(null);
 const pending=useRef<AbortController|null>(null);
 const version=useRef(0);
 const callback=useRef(onLogin);callback.current=onLogin;

 const accept=(result:Login)=>{
  current.current=result;setLogin(result);
  if(result.state==='success'){
   setCode('');setPhone('');
   window.dispatchEvent(new Event('bilifetch-account-changed'));
   callback.current();
  }
 };
 const cancel=()=>{
  version.current++;pending.current?.abort();pending.current=null;
  if(current.current)void request(`/xiaohongshu/login/${current.current.id}/cancel`,'POST').catch(()=>{});
  current.current=null;setLogin(null);setError('');setPollFailed(false);setBusy(false);setCode('');
 };
 const close=()=>{cancel();setPhone('');setOpen(false)};
 const run=async(path:string,body?:unknown)=>{
  pending.current?.abort();
  const controller=new AbortController();pending.current=controller;
  const generation=++version.current;
  setBusy(true);setError('');setPollFailed(false);
  try{
   const result=await request<Login>(path,'POST',body,controller.signal);
   if(generation!==version.current)return;
   accept(result);
   if(result.state==='waiting_code')setRetryAt(Date.now()+60_000);
  }catch(e){if(generation===version.current&&!controller.signal.aborted)setError(e instanceof Error?e.message:'小红书登录失败')}
  finally{if(generation===version.current){setBusy(false);pending.current=null}}
 };
 const generate=()=>{cancel();void run('/xiaohongshu/login')};
 const sendCode=()=>{cancel();void run('/xiaohongshu/login/sms',{phone})};
 useEffect(()=>{
  if(open&&method==='qr')generate();
  return()=>{version.current++;pending.current?.abort()};
 },[open,method]);
 useEffect(()=>()=>{
  if(current.current)void request(`/xiaohongshu/login/${current.current.id}/cancel`,'POST').catch(()=>{});
 },[]);
 useEffect(()=>{
  if(!open)return;
  const timer=setInterval(()=>setNow(Date.now()),1000);
  return()=>clearInterval(timer);
 },[open]);
 useEffect(()=>{
  if(login&&login.state!=='success'&&login.state!=='expired'&&now>=Date.parse(login.expiresAt)){
   accept({...login,state:'expired',message:method==='qr'?'二维码已过期，请重新生成':'验证码登录已过期，请重新获取'});
  }
 },[now,login,method]);
 useEffect(()=>{
  if(!open||method!=='qr'||!login||pollFailed||!['waiting_scan','waiting_confirm'].includes(login.state))return;
  const session=login;const generation=version.current;let stopped=false;let timer:ReturnType<typeof setTimeout>;
  const controller=new AbortController();
  const check=async()=>{
   try{
    const result=await request<Login>(`/xiaohongshu/login/${session.id}/poll`,'POST',undefined,controller.signal);
    if(stopped||generation!==version.current)return;
    accept(result);setError('');
    if(result.state==='success'||result.state==='expired')return;
    timer=setTimeout(check,2500);
   }catch(e){if(!stopped&&generation===version.current){setError(e instanceof Error?e.message:'查询扫码状态失败');setPollFailed(true)}}
  };
  timer=setTimeout(check,2200);
  return()=>{stopped=true;clearTimeout(timer);controller.abort()};
 },[open,method,login?.id,login?.state,pollFailed]);
 const retrySeconds=Math.max(0,Math.ceil((retryAt-now)/1000));
 const switchMethod=(value:'qr'|'sms')=>{if(value===method)return;cancel();setMethod(value)};

 return <>
  <Button color="primary" startContent={<Smartphone size={14}/>} onPress={()=>{setMethod('qr');setOpen(true)}}>扫码登录</Button>
  <Modal isOpen={open} onClose={close} size="md" classNames={{base:'app-modal'}}>
   <ModalContent>
    <ModalHeader><h2>登录小红书</h2></ModalHeader>
    <ModalBody data-slot="body">
     {login?.state!=='success'&&<div className="flex gap-2" aria-label="登录方式">
      <Button size="sm" aria-pressed={method==='qr'} variant={method==='qr'?'flat':'light'} color={method==='qr'?'primary':'default'} onPress={()=>switchMethod('qr')}>扫码登录</Button>
      <Button size="sm" aria-pressed={method==='sms'} variant={method==='sms'?'flat':'light'} color={method==='sms'?'primary':'default'} onPress={()=>switchMethod('sms')}>短信验证码</Button>
     </div>}
     {login?.state==='success'?<div className="qr-login-area"><div className="qr-success"><CheckCircle2 size={66}/><h3>登录成功</h3></div></div>:method==='qr'?<div className="qr-login-area">
      {busy?<div className="qr-placeholder"><RefreshCw className="animate-spin" size={30}/><p>正在申请小红书二维码…</p></div>:login?.url?<>
       <div className={`qr-code ${login.state==='expired'?'expired':''}`}>
        <QRCodeSVG value={login.url} size={208} level="M" marginSize={4} title="小红书扫码登录二维码"/>
        {login.state==='expired'&&<div><p>二维码已过期</p><Button size="sm" color="primary" onPress={generate}>刷新二维码</Button></div>}
       </div>
       <p className="qr-login-status"><Smartphone size={16}/>{login.message}</p>
      </>:<p className="muted">申请二维码后，使用小红书 App 扫码并确认登录。</p>}
     </div>:<form className="flex flex-col gap-4" onSubmit={e=>{e.preventDefault();if(login?.state==='waiting_code')void run(`/xiaohongshu/login/${login.id}/verify`,{phone,code})}}>
      <Input label="手机号" labelPlacement="outside-top" variant="bordered" type="tel" autoComplete="tel-national" placeholder="中国大陆手机号（+86）" maxLength={11} value={phone} isDisabled={busy} onValueChange={value=>{if(current.current)cancel();setPhone(value.replace(/\D/g,''))}}/>
      <div className="flex items-end gap-2">
       <Input label="短信验证码" labelPlacement="outside-top" variant="bordered" inputMode="numeric" autoComplete="one-time-code" maxLength={6} value={code} isDisabled={busy||login?.state!=='waiting_code'} onValueChange={value=>setCode(value.replace(/\D/g,''))}/>
       <Button variant="bordered" isLoading={busy&&!login} isDisabled={busy||!/^1[3-9]\d{9}$/.test(phone)||retrySeconds>0} onPress={sendCode}>{retrySeconds>0?`${retrySeconds} 秒后重发`:'获取验证码'}</Button>
      </div>
      {login&&<p className="muted" role="status">{login.message}</p>}
      <Button type="submit" color="primary" isLoading={busy&&!!login} isDisabled={busy||login?.state!=='waiting_code'||!/^\d{6}$/.test(code)}>验证并登录</Button>
     </form>}
     {error&&<ErrorNote>{error}</ErrorNote>}
     {login?.state!=='success'&&<p className="muted platform-account-note">请在小红书 App 完成扫码确认；如提示额外验证，请先在 App 完成后重试。</p>}
    </ModalBody>
    <ModalFooter data-slot="footer">
     {method==='qr'&&login?.state!=='success'&&<Button variant="light" startContent={<RefreshCw size={14}/>} isDisabled={busy} onPress={generate}>重新生成</Button>}
     <Button color="primary" onPress={close}>{login?.state==='success'?'完成':'关闭'}</Button>
    </ModalFooter>
   </ModalContent>
  </Modal>
 </>;
}
