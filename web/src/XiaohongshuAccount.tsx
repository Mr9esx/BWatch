import {useEffect,useRef,useState} from 'react';
import {LogOut,UserRound} from 'lucide-react';
import {Button} from '@heroui/react';
import {request} from './api';
import {useApp} from './context';
import XiaohongshuLogin from './XiaohongshuLogin';
import {PlatformBadge} from './components';

type Account={state:'logged_in'|'logged_out'|'unavailable';message:string;id?:string;redID?:string;name?:string;avatar?:string;profileMessage?:string};

export default function XiaohongshuAccount(){
 const {health}=useApp();const [account,setAccount]=useState<Account|null>(null);const [avatarFailed,setAvatarFailed]=useState(false);const [busy,setBusy]=useState(false);const [refreshing,setRefreshing]=useState(false);const [message,setMessage]=useState('');const alive=useRef(true);
 const refresh=async()=>{setRefreshing(true);try{const result=await request<Account>('/xiaohongshu/account');if(alive.current){setAccount(result);setMessage('')}}catch{if(alive.current){setAccount({state:'unavailable',message:'服务未连接'});setMessage('')}}finally{if(alive.current)setRefreshing(false)}};
 useEffect(()=>{alive.current=true;if(health?.ok)void refresh();return()=>{alive.current=false}},[health?.ok]);
 useEffect(()=>setAvatarFailed(false),[account?.avatar]);
 const logout=async()=>{setBusy(true);try{await request<{ok:boolean}>('/xiaohongshu/logout','POST');await refresh();setMessage('已退出小红书登录')}catch(e){setMessage(e instanceof Error?e.message:'退出登录失败')}finally{setBusy(false)}};
 return <section className="panel settings-section platform-account-card" aria-label="小红书账号管理">
  <div className="platform-account-heading"><span className="account-avatar">{account?.avatar&&!avatarFailed?<img src={account.avatar} alt={`${account.name||'小红书账号'}的头像`} referrerPolicy="no-referrer" onError={()=>setAvatarFailed(true)}/>:<UserRound size={20}/>}</span><div className="platform-account-identity"><h2>{account?.name||'小红书账号'}</h2><p className="muted">{account?.redID?`小红书号：${account.redID}`:account?.state==='logged_in'?'账号资料暂未获取':'扫码登录后读取创作者笔记并归档'}</p></div><PlatformBadge platform="xiaohongshu"/></div>
  <div className={`account-status platform-account-status ${account?.state||'loading'}`} role="status" title={message||account?.message}><i aria-hidden="true"/><span>{message||account?.message||'正在读取账号状态…'}</span></div>
  <div className="platform-account-actions">{account?.state==='logged_in'?<Button variant="bordered" isLoading={busy} startContent={<LogOut size={14}/>} onPress={logout}>退出登录</Button>:<XiaohongshuLogin onLogin={()=>void refresh()}/>}<Button variant="bordered" isLoading={refreshing} isDisabled={busy} onPress={()=>void refresh()}>刷新状态</Button></div>
  <p className="muted platform-account-note">{account?.profileMessage||'支持小红书 App 扫码或短信验证码登录，登录凭据加密保存在本地。'}</p>
 </section>
}
