import assert from 'node:assert/strict';
import {mkdir} from 'node:fs/promises';
import {fileURLToPath,pathToFileURL} from 'node:url';
import {dirname,join} from 'node:path';
import {build} from 'esbuild';
import React from 'react';
import {renderToStaticMarkup} from 'react-dom/server';

const root=dirname(dirname(fileURLToPath(import.meta.url)));
const output=join(root,'node_modules/.cache/library-folders/check.mjs');
await mkdir(dirname(output),{recursive:true});
await build({stdin:{contents:"export {default as Library,getLibraryGroups,selectLibraryTasks,LibraryItem} from './src/Library';export {AppContext} from './src/context';export {emptyState} from './src/types';export {pageHash,readRoute} from './src/navigation';",resolveDir:root},outfile:output,bundle:true,platform:'node',format:'esm',packages:'external',jsx:'automatic',logLevel:'error'});
const {Library,getLibraryGroups,selectLibraryTasks,LibraryItem,AppContext,emptyState,pageHash,readRoute}=await import(pathToFileURL(output));
const creator=(id,name,platform)=>({id,name,platform,url:`https://example.invalid/${id}`,interval:10,quality:'best',history:false,watchVideos:true,watchDynamics:true,enabled:true,createdAt:'2026-10-22T00:00:00Z',lastChecked:null,lastError:'',initialized:true,dynamicInitialized:true,seen:{},seenDynamics:{}});
const creators=[creator('one','创作者甲','bilibili'),creator('two','创作者乙','xiaohongshu')];
const task=(id,creatorId,creatorName,platform,kind='video',status='completed')=>({id,creatorId,creatorName,platform,kind,videoId:id,title:`内容标题-${id}`,url:`https://example.invalid/${id}`,thumbnail:'',duration:30,quality:'best',status,progress:100,speed:'',eta:'',error:'',file:`/archive/${id}/content`,bytes:100,createdAt:'2026-10-21T00:00:00Z',finishedAt:status==='completed'?'2026-10-22T00:00:00Z':null});
const tasks=[task('甲视频','one','旧名称','bilibili'),task('甲图文','one','旧名称','bilibili','image_post'),task('乙视频','two','创作者乙','xiaohongshu'),task('遗留归档','removed','已移除创作者','bilibili'),task('未完成','one','旧名称','bilibili','video','queued')];
function render(scope={creatorId:null,platform:'all'},search=''){
 const model={state:{...emptyState,creators,tasks},health:null,search,setSearch(){},page:'library',libraryScope:scope,navigate(){},openAdd(){},act:async()=>true,preview(){},notify(){},confirm(){}};
 return renderToStaticMarkup(React.createElement(AppContext.Provider,{value:model},React.createElement(Library)));
}

const folders=render();
assert.match(folders,/role="tablist"[^>]*aria-label="内容库平台"|aria-label="内容库平台"[^>]*role="tablist"/,'平台筛选必须使用组件库 Tab');
assert.ok(!folders.includes('筛选内容库平台'),'不应继续显示平台下拉框');
assert.ok(!folders.includes('内容标题-甲视频'),'进入内容库时应先选择创作者，不能直接渲染归档文件');
assert.match(folders,/href="#library\?creator=one"/,'创作者文件夹必须能打开对应的归档路径');
assert.match(folders,/href="#library\?creator=two"/,'不同创作者必须有独立的文件夹入口');
assert.match(folders,/href="#library\?creator=removed"/,'已移除订阅的归档仍须可以从文件夹进入');
const bilibili=render({creatorId:null,platform:'bilibili'});
assert.ok(!bilibili.includes('重置筛选'),'平台Tab是浏览范围，不应产生额外重置筛选行');
assert.match(bilibili,/href="#library\?platform=bilibili&amp;creator=one"/,'打开平台内的文件夹必须保留平台范围');
assert.ok(!bilibili.includes('创作者乙'),'平台筛选不能混入其他平台的创作者文件夹');
const searched=render({creatorId:null,platform:'all'},'创作者乙');
assert.match(searched,/清除创作者筛选/,'搜索时应提供紧凑的清除筛选入口');
assert.match(searched,/href="#library\?creator=two"/,'根目录搜索应能定位创作者文件夹');
assert.ok(!searched.includes('href="#library?creator=one"'),'根目录搜索不能继续展示无关的创作者');

const detail=render({creatorId:'one',platform:'all'});
assert.match(detail,/返回创作者列表/,'创作者内容页必须提供明确的返回按钮');
assert.match(render({creatorId:'one',platform:'bilibili'}),/href="#library\?platform=bilibili"[^>]*>.*?返回创作者列表/s,'返回按钮必须保留原平台');
assert.ok(!detail.includes('创作者乙'),'文件夹内不应保留其他创作者侧栏');
assert.match(detail,/href="#library"/,'文件夹内必须提供返回根目录的路径');
assert.match(detail,/搜索该创作者的归档内容/,'文件夹搜索必须明确限定在当前创作者');
assert.match(detail,/全部类型/,'文件夹内必须保留内容类型筛选');
assert.match(detail,/内容排序/,'文件夹内必须保留归档排序');
const removed=render({creatorId:'removed',platform:'all'});
assert.match(removed,/已移除创作者/,'没有订阅资料的文件夹应继续显示任务保存的作者名称');

const groups=getLibraryGroups(tasks,creators);
assert.equal(groups.length,3,'未完成的任务不能创建额外归档文件夹');
const first=groups.find(group=>group.id==='one');
assert.deepEqual([first.name,first.tasks.length,first.videos,first.dynamics,first.size],['创作者甲',2,1,1,200],'文件夹应使用最新作者名称，且只统计已完成的归档');
assert.deepEqual(readRoute(pageHash('library',{platform:'bilibili',creatorId:'one'})),{page:'library',libraryScope:{creatorId:'one',platform:'bilibili'}},'已有订阅卡片的内容库直达链接必须继续定位到对应文件夹');
console.log('内容库文件夹检查通过：根目录、作者范围、平台、搜索、遗留归档与直达链接');

const publications=[{...task('old','one','UP','bilibili'),publishedAt:'2024-01-01T00:00:00Z'},{...task('missing','one','UP','bilibili')},{...task('new','one','UP','bilibili'),publishedAt:'2026-01-01T00:00:00Z'},{...task('invalid','one','UP','bilibili'),publishedAt:'invalid'}];
assert.deepEqual(selectLibraryTasks(publications,{sort:'published-new'}).map(t=>t.id),['new','old','missing','invalid'],'最新发布排序必须独立于归档时间，未知日期置后');
assert.deepEqual(selectLibraryTasks(publications,{sort:'published-old'}).map(t=>t.id),['old','new','missing','invalid'],'最早发布排序仍须把未知日期置后');
function renderItem(item){return renderToStaticMarkup(React.createElement(AppContext.Provider,{value:{state:{...emptyState,creators,tasks:publications},preview(){}}},React.createElement(LibraryItem,{task:item})));}
assert.match(renderItem(publications[0]),/发布于 2024/,'发布时间必须显示年份以区分跨年内容');
assert.match(renderItem(publications[1]),/发布时间待补齐/,'缺失发布时间必须明确显示');
console.log('发布时间排序检查通过：正序、倒序、缺失与无效日期');
