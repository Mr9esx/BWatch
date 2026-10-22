import {ShieldCheck} from 'lucide-react';
import {PageHeading} from './ContentPages';
import BilibiliAccount from './BilibiliAccount';
import XiaohongshuAccount from './XiaohongshuAccount';

export default function PlatformManagement(){
 return <div className="page-enter platform-management"><PageHeading title="平台管理"/><div className="platform-management-intro"><ShieldCheck size={20}/><p>每个平台的登录状态独立管理。添加订阅前，需要先登录对应平台账号。</p></div><div className="platform-account-grid"><BilibiliAccount/><XiaohongshuAccount/></div></div>
}
