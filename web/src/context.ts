import {createContext,useContext} from 'react';
import type {State,Health,Page,Task} from './types';
export type AppModel={state:State;health:Health|null;search:string;setSearch:(value:string)=>void;page:Page;navigate:(page:Page)=>void;openAdd:()=>void;act:(path:string,method?:string,body?:unknown,message?:string)=>Promise<boolean>;preview:(task:Task)=>void;notify:(message:string,error?:boolean)=>void;confirm:(title:string,description:string,action:()=>Promise<boolean>)=>void};
export const AppContext=createContext<AppModel|null>(null);
export function useApp(){const value=useContext(AppContext);if(!value)throw new Error('App context missing');return value;}
