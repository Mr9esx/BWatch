import {useEffect,useState} from 'react';
import {UserRound} from 'lucide-react';
import {Button} from '@heroui/react';
import {request} from './api';
import {useApp} from './context';
import BilibiliLogin from './BilibiliLogin';

type Account={state:'logged_in'|'logged_out'|'expired'|'unavailable';uid?:number;name?:string;avatar?:string;message?:string};
export default function BilibiliAccount(){
 const {state,health}=useApp();const [account,setAccount]=useState<Account|null>(null);const [avatarFailed,setAvatarFailed]=useState(false);const [refreshing,setRefreshing]=useState(false);
 useEffect(()=>{
  if(!health?.ok)return;
  setAccount(null);
  let active=true,pending=false;
  const refresh=async()=>{if(pending)return;pending=true;try{const result=await request<Account>('/bilibili/account');if(active)setAccount(result)}catch{if(active)setAccount(current=>({...current,state:'unavailable',message:'服务未连接'}))}finally{pending=false}};
  void refresh();const timer=setInterval(refresh,60000);window.addEventListener('bilifetch-account-changed',refresh);
  return()=>{active=false;clearInterval(timer);window.removeEventListener('bilifetch-account-changed',refresh)};
 },[health?.ok,state.settings.proxyURL]);
 useEffect(()=>setAvatarFailed(false),[account?.avatar]);
 const refreshAccount=async()=>{if(refreshing)return;setRefreshing(true);try{setAccount(await request<Account>('/bilibili/account'))}catch{setAccount({state:'unavailable',message:'服务未连接'})}finally{setRefreshing(false)}};
 return <section className="panel settings-section platform-account-card" aria-label="Bilibili 账号管理">
  <div className="platform-account-heading"><span className="account-avatar">{account?.avatar&&!avatarFailed?<img src={account.avatar} alt="账号头像" referrerPolicy="no-referrer" onError={()=>setAvatarFailed(true)}/>:<UserRound size={21}/>}</span><div><h2>Bilibili</h2><p className={account?.name?'platform-account-name':'muted'}>{account?.name||'管理登录状态和订阅授权'}</p></div></div>
  <div className={`account-status platform-account-status ${account?.state||'loading'}`} role="status" title={account?.message}><i aria-hidden="true"/><span>{account?.state==='logged_in'?`已登录 · ${account.name||'Bilibili 账号'}`:account?.message||'正在读取账号状态…'}</span>{account?.uid&&<small>UID {account.uid}</small>}</div>
  <div className="platform-account-actions"><BilibiliLogin compact label={account?.state==='logged_in'?'切换账号':account?.state==='expired'?'重新登录':'扫码登录'}/><Button variant="bordered" isLoading={refreshing} onPress={refreshAccount}>刷新状态</Button></div>
 </section>
}
