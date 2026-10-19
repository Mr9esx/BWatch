export async function request<T>(path:string,method='GET',body?:unknown,signal?:AbortSignal):Promise<T>{
 const response=await fetch('/api'+path,{method,headers:body?{'Content-Type':'application/json'}:undefined,body:body?JSON.stringify(body):undefined,signal});
 const data=await response.json().catch(error=>{if(signal?.aborted)throw error;throw new Error('服务返回了无效响应，请检查 Go 服务是否启动')});
 if(!response.ok)throw new Error(data.error||`请求失败 (${response.status})`);
 return data as T;
}
