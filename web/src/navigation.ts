import type {Page} from './types';
import {parsePlatform,type PlatformFilter} from './platforms';

export type LibraryView='all'|'creators';
export type LibraryScope={creatorId:string|null;platform:PlatformFilter;view:LibraryView};
export type LibraryDestination={creatorId?:string|null;platform?:PlatformFilter;view?:LibraryView};
const pages:Page[]=['overview','creators','platforms','tasks','library','logs','settings'];

export function readRoute(hash:string):{page:Page;libraryScope:LibraryScope}{
 const [path,query='']=hash.replace(/^#/,'').split('?');
 const page=pages.includes(path as Page)?path as Page:'overview';
 const params=new URLSearchParams(query);
 const creatorId=page==='library'?params.get('creator')||null:null;
 return {page,libraryScope:{creatorId,platform:page==='library'?parsePlatform(params.get('platform')):'all',view:page==='library'&&(creatorId||params.get('view')==='creators')?'creators':'all'}};
}

export function pageHash(page:Page,destination:LibraryDestination={}):string{
 const params=new URLSearchParams();
 if(page==='library'){
  if(destination.platform&&destination.platform!=='all')params.set('platform',destination.platform);
  if(destination.view==='creators'&&!destination.creatorId)params.set('view','creators');
  if(destination.creatorId)params.set('creator',destination.creatorId);
 }
 return `#${page}${params.size?'?'+params.toString():''}`;
}
