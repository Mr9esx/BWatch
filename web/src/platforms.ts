import type {Creator} from './types';

export type Platform=Creator['platform'];
export type PlatformFilter=Platform|'all';
export const platforms=[{id:'bilibili',label:'Bilibili'},{id:'xiaohongshu',label:'小红书'}] as const;
export const platformLabel=(platform?:Platform)=>platforms.find(item=>item.id===platform)?.label||'未知平台';
export const matchesPlatform=(item:{platform?:Platform},platform:PlatformFilter)=>platform==='all'||item.platform===platform;
export const parsePlatform=(value:string|null):PlatformFilter=>platforms.some(platform=>platform.id===value)?value as Platform:'all';
