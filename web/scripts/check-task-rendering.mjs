import assert from 'node:assert/strict';
import {mkdir} from 'node:fs/promises';
import {fileURLToPath,pathToFileURL} from 'node:url';
import {dirname,join} from 'node:path';
import {build} from 'esbuild';
import React from 'react';
import {renderToStaticMarkup} from 'react-dom/server';

const root=dirname(dirname(fileURLToPath(import.meta.url)));
const output=join(root,'node_modules/.cache/task-rendering/check.mjs');
await mkdir(dirname(output),{recursive:true});
await build({stdin:{contents:"export {Tasks} from './src/ContentPages';export {SettingsPage} from './src/SystemPages';export {AppContext} from './src/context';export {emptyState} from './src/types';",resolveDir:root},outfile:output,bundle:true,platform:'node',format:'esm',packages:'external',jsx:'automatic',logLevel:'error'});
const {Tasks,SettingsPage,AppContext,emptyState}=await import(pathToFileURL(output));
const tasks=Array.from({length:1000},(_,i)=>({id:`task-${i}`,creatorId:'one',creatorName:'测试创作者',platform:'bilibili',kind:'video',videoId:String(i),title:`任务标题[${i}]`,url:'https://example.invalid/',thumbnail:'',duration:30,quality:'best',status:'completed',progress:100,speed:'',eta:'',error:'',file:`/archive/${i}.mp4`,bytes:100,createdAt:'2026-10-22T00:00:00Z',finishedAt:'2026-10-22T00:00:00Z'}));
function render(search='',overrides={},Component=Tasks){
 const model={state:{...emptyState,tasks,...overrides},health:null,search,setSearch(){},page:'tasks',libraryScope:{creatorId:null,platform:'all'},navigate(){},openAdd(){},act:async()=>true,preview(){},notify(){},confirm(){}};
 return renderToStaticMarkup(React.createElement(AppContext.Provider,{value:model},React.createElement(Component)));
}
const start=performance.now();const html=render();const rows=(html.match(/<tr\b/g)||[]).length;
assert.ok(rows>1&&rows<40,`1000 条任务首屏应仅挂载可视区域，实际渲染 ${rows} 行`);
assert.ok(html.includes('任务标题[999]'),'首屏应保持最新任务优先');
assert.ok(!html.includes('任务标题[0]'),'未滚动到的远端行不应提前挂载');
assert.match(html,/共 1000 条任务/,'虚拟渲染不能截断任务总量');
const found=render('任务标题[0]');
assert.ok(found.includes('任务标题[0]'),'搜索必须能找到首屏以外的任务');
assert.match(found,/共 1 条任务/,'筛选总量应基于完整数据');
assert.ok(!found.includes('任务标题[999]'),'筛选不能混入其他任务');
console.log(`下载任务首屏检查通过：1000 条任务挂载 ${rows} 行，含完整数据搜索，${Math.round(performance.now()-start)}ms`);
const queue=tasks.slice(0,2).map(t=>({...t,status:'queued'}));
const batch={tasks:queue,nextDownloadAt:'2099-01-01T00:00:00Z',settings:{...emptyState.settings,concurrency:4}};
assert.match(render('',batch),/下一批不早于/,'批量并发仍须显示下一批的启动时间');
assert.match(render('',batch),/到点启动最多 4 条/,'任务页必须说明每批任务数');
assert.match(render('',batch,SettingsPage),/批次之间不补位/,'设置页必须说明按间隔分批启动');
console.log('批次下载提示检查通过：启动时间、任务数与等待规则');
