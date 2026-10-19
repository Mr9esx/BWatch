import {useEffect,useState} from 'react';
import {UserRound} from 'lucide-react';
import {request} from './api';
import {useApp} from './context';
import BilibiliLogin from './BilibiliLogin';

type Account={state:'logged_in'|'logged_out'|'expired'|'unavailable';uid?:number;name?:string;avatar?:string;message?:string};
export default function BilibiliAccount(){
 const {state,health}=useApp();const [account,setAccount]=useState<Account|null>(null);const [avatarFailed,setAvatarFailed]=useState(false);
 useEffect(()=>{
  if(!health?.ok)return;
  setAccount(null);
  let active=true,pending=false;
  const refresh=async()=>{if(pending)return;pending=true;try{const result=await request<Account>('/bilibili/account');if(active)setAccount(result)}catch{if(active)setAccount(current=>({...current,state:'unavailable',message:'服务未连接'}))}finally{pending=false}};
  void refresh();const timer=setInterval(refresh,60000);window.addEventListener('bilifetch-account-changed',refresh);
  return()=>{active=false;clearInterval(timer);window.removeEventListener('bilifetch-account-changed',refresh)};
 },[health?.ok,state.settings.cookiesFile,state.settings.proxyURL]);
 useEffect(()=>setAvatarFailed(false),[account?.avatar]);
 return <section className="sidebar-account" aria-label="B 站登录账号">
  <div className="account-identity"><span className="account-avatar">{account?.avatar&&!avatarFailed?<img src={account.avatar} alt="账号头像" referrerPolicy="no-referrer" onError={()=>setAvatarFailed(true)}/>:<UserRound size={21}/>}</span><div><strong title={account?.name}>{account?.name||'B 站账号'}</strong>{account?.uid&&<span className="account-uid" title={`UID ${account.uid}`}>UID {account.uid}</span>}</div></div>
  <div className="account-actions"><div className={`account-status ${account?.state||'loading'}`} role="status" title={account?.message}><i aria-hidden="true"/><span>{account?.message||'正在读取账号…'}</span></div><BilibiliLogin compact label={account?.state==='logged_in'?'切换账号':account?.state==='expired'?'重新扫码':'扫码登录'}/></div>
 </section>
}
