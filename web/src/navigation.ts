import type {Page} from './types';
import {parsePlatform,type PlatformFilter} from './platforms';

export type LibraryScope={creatorId:string|null;platform:PlatformFilter};
export type LibraryDestination={creatorId?:string|null;platform?:PlatformFilter};
const pages:Page[]=['overview','creators','platforms','tasks','library','logs','settings'];

export function readRoute(hash:string):{page:Page;libraryScope:LibraryScope}{
 const [path,query='']=hash.replace(/^#/,'').split('?');
 const page=pages.includes(path as Page)?path as Page:'overview';
 const params=new URLSearchParams(query);
 return {page,libraryScope:{creatorId:page==='library'?params.get('creator')||null:null,platform:page==='library'?parsePlatform(params.get('platform')):'all'}};
}

export function pageHash(page:Page,destination:LibraryDestination={}):string{
 const params=new URLSearchParams();
 if(page==='library'){
  if(destination.platform&&destination.platform!=='all')params.set('platform',destination.platform);
  if(destination.creatorId)params.set('creator',destination.creatorId);
 }
 return `#${page}${params.size?'?'+params.toString():''}`;
}
