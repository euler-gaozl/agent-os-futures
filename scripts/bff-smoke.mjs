// Real owner executable and built Web; downstream HTTP peers are synthetic.
// No user configuration, database, provider or model calls are used.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import http from 'node:http';
import {spawn,execFileSync} from 'node:child_process';
import {randomBytes,createHash} from 'node:crypto';
const root=path.resolve(process.argv[process.argv.indexOf('--package')+1]||'');
const manifest=JSON.parse(fs.readFileSync(path.join(root,'manifest.json'),'utf8'));
assert.ok(['control','futures'].includes(manifest.role));
const role=manifest.role, suffix=process.platform==='win32'?'.exe':'';
const fixture=fs.mkdtempSync(path.join(path.dirname(root),'bff-fixture-'));
const serviceToken=randomBytes(32).toString('hex'), userToken=randomBytes(32).toString('hex');
const wrongAdmin=randomBytes(32).toString('hex');
const requests=[];let failToolhub=false,instanceSeen=false;
const json=(res,status,body)=>{res.writeHead(status,{'Content-Type':'application/json'});res.end(JSON.stringify(body))};
const upstream=http.createServer(async(req,res)=>{
 try{
  assert.equal(req.headers.authorization,'Bearer '+serviceToken);
  const chunks=[];for await(const c of req)chunks.push(c);
  const text=Buffer.concat(chunks).toString();
  if(req.url==='/v1/instances'){
   const body=JSON.parse(text);assert.equal(body.role,role);assert.equal(body.pid,child.pid);instanceSeen=true;return json(res,200,{});
  }
  requests.push({method:req.method,url:req.url,key:req.headers['idempotency-key'],body:text?JSON.parse(text):null});
  if(req.url.startsWith('/v1/catalog'))return json(res,200,{items:[{kind:'agent',id:'fixture',revision:1,spec:{name:'Fixture'}}],next_cursor:''});
  if(req.url.startsWith('/v1/assets/catalog'))return json(res,failToolhub?503:200,{items:[],next_cursor:''});
  if(req.url==='/v1/published/futures')return json(res,200,[{id:'fixture',revision:1,name:'Fixture research'}]);
  if(req.url==='/v1/status')return json(res,200,{deepseek_configured:false,indexed_documents:0});
  if(req.url==='/v1/apps/futures/runs'&&req.method==='POST')return json(res,202,{id:'fixture-run',state:'queued'});
  if(req.url==='/v1/apps/futures/runs'&&req.method==='GET')return json(res,200,[{id:'fixture-run',state:'completed',metadata:{commodity:'甲醇',internal:'hidden'}}]);
  if(req.url==='/v1/apps/futures/runs/fixture-run')return json(res,200,{id:'fixture-run',state:'completed',metadata:{commodity:'甲醇',internal:'hidden'},result:'Synthetic only'});
  if(req.url==='/v1/apps/futures/runs/fixture-run/cancel')return json(res,200,{state:'cancelled'});
  if(req.url==='/artifacts/fixture?offset=1'){res.writeHead(206,{'Content-Type':'text/plain','Content-Disposition':'attachment; filename="fixture.txt"','X-Artifact-Media-Type':'text/plain'});return res.end('retained fixture');}
  return json(res,404,{error:'unexpected fixture route'});
 }catch(error){res.destroy(error);fixtureError=error}
});
let fixtureError;
await new Promise(resolve=>upstream.listen(0,'127.0.0.1',resolve));
const upstreamURL='http://127.0.0.1:'+upstream.address().port;
const reservation=http.createServer();await new Promise(resolve=>reservation.listen(0,'127.0.0.1',resolve));
const port=reservation.address().port;await new Promise(resolve=>reservation.close(resolve));
const url='http://127.0.0.1:'+port;
const example=JSON.parse(fs.readFileSync(path.join(root,'deploy','service.example.json'),'utf8'));
const webKey=role==='control'?'WEB_DIR':'FUTURES_WEB_DIR';
const config={...example,LISTEN:'127.0.0.1:'+port,SERVICE_TOKEN:serviceToken,SERVICE_PEERS:JSON.stringify({[role]:createHash('sha256').update(serviceToken).digest('hex')}),ADMIN_TOKEN:role==='control'?userToken:wrongAdmin,BUSINESS_TOKEN:userToken,DEV_LOCAL:'0',REGISTRY_URL:upstreamURL,RUNTIME_URL:upstreamURL,TOOLHUB_URL:upstreamURL,KNOWLEDGE_URL:upstreamURL,STORAGE_URL:upstreamURL,SOURCES_URL:upstreamURL,[webKey]:path.resolve(root,example[webKey]),INSTANCE_ID:'fixture-'+role,DATABASE_URL:'invalid://must-never-connect',HTTP_MAX_CONCURRENT:'4'};
const configPath=path.join(fixture,'service.json');fs.writeFileSync(configPath,JSON.stringify(config));
const env={};for(const key of ['SystemRoot','SYSTEMROOT','WINDIR','SystemDrive','PATH','TEMP','TMP','HOME'])if(process.env[key])env[key]=process.env[key];
const log=fs.openSync(path.join(fixture,'process.log'),'w');
const child=spawn(path.join(root,'bin',role+suffix),['--config',configPath],{env,cwd:fixture,stdio:['ignore',log,log],windowsHide:true});
const exited=new Promise(resolve=>{child.once('exit',(code,signal)=>resolve({code,signal}));child.once('error',()=>resolve({spawn_failed:true}))});
child.once('error',error=>{fixtureError=error});
const sleep=ms=>new Promise(resolve=>setTimeout(resolve,ms));
const request=(p,token=userToken,options={})=>fetch(url+p,{...options,headers:{...(token?{Authorization:'Bearer '+token}:{}),...options.headers},signal:AbortSignal.timeout(5000)});
let success=false;
try{
 let ready=false;
 for(let i=0;i<100;i++){
  if(child.exitCode!==null||fixtureError)throw fixtureError||Error('BFF exited before readiness');
  try{if((await request('/readyz')).status===200){ready=true;break}}catch{}
  await sleep(50);
 }
 assert.ok(ready,'bounded readiness');
 for(const p of ['/readyz','/metrics',role==='control'?'/api/catalog':'/api/agents']){
  assert.equal((await request(p,null)).status,401);
  assert.equal((await request(p,serviceToken)).status,401,'service token is not the UI token');
 }
 if(role==='futures')assert.equal((await request('/api/agents',wrongAdmin)).status,401,'business token must win over ADMIN_TOKEN');
 const index=await request('/',null);assert.equal(index.status,200);const html=await index.text();
 assert.ok(html.includes(role==='control'?'Agent OS':'研序'));
 const asset=html.match(/src="([^"]+\.js)"/);assert.ok(asset,'built entry asset');
 const entry=await request(asset[1],null);assert.equal(entry.status,200);
 const bytes=await entry.text();assert.ok(!bytes.includes(serviceToken)&&!bytes.includes(userToken));
 assert.equal((await request(asset[1],null,{method:'POST'})).status,401);
 execFileSync(path.join(root,'bin','healthcheck'+suffix),[],{env:{...env,AGENTOS_CONFIG:configPath,HEALTH_URL:url+'/readyz'},windowsHide:true,stdio:'pipe',timeout:5000});
 if(role==='control'){
  const catalog=await request('/api/catalog?kind=agent&limit=2');assert.equal(catalog.status,200);assert.equal((await catalog.json()).items[0].id,'fixture');
  const before=requests.length;assert.equal((await request('/api/catalog?limit=500')).status,400);assert.equal(requests.length,before);
  failToolhub=true;const partial=await(await request('/api/catalog')).json();assert.equal(partial.services.toolhub,false);assert.equal(partial.next_cursor,'');
  const raw=await request('/api/artifacts/fixture?offset=1');assert.equal(raw.status,206);assert.equal(raw.headers.get('content-disposition'),'attachment; filename="fixture.txt"');assert.equal(await raw.text(),'retained fixture');
 }else{
  assert.equal((await(await request('/api/agents')).json())[0].id,'fixture');
  const task={agent:{id:'fixture',revision:1},commodity:'甲醇',period:'2026-10-01 至 2026-10-12',question:'供给库存资料核验，不预测'};
  let count=requests.length;
  assert.equal((await request('/api/tasks',userToken,{method:'POST',body:JSON.stringify(task)})).status,400);assert.equal(requests.length,count);
  const submit=await request('/api/tasks',userToken,{method:'POST',headers:{'Content-Type':'application/json','Idempotency-Key':'fixture-stable'},body:JSON.stringify(task)});assert.equal(submit.status,202);
  const sent=requests.find(r=>r.method==='POST'&&r.url==='/v1/apps/futures/runs');assert.equal(sent.key,'fixture-stable');assert.deepEqual(sent.body.agent,task.agent);assert.equal(sent.body.metadata.commodity,'甲醇');assert.ok(sent.body.task.includes(task.question));
  const rows=await(await request('/api/tasks')).json();assert.equal(rows[0].commodity,'甲醇');assert.ok(!('metadata'in rows[0]));
  const detail=await(await request('/api/tasks/fixture-run')).json();assert.equal(detail.commodity,'甲醇');assert.ok(!('metadata'in detail));
  assert.equal((await request('/api/tasks/fixture-run/cancel',userToken,{method:'POST',body:'{}',headers:{'Idempotency-Key':'fixture-cancel'}})).status,200);
  const readiness=await(await request('/api/readiness')).json();assert.equal(readiness.model_configured,false);assert.equal(readiness.knowledge_available,true);
  count=requests.length;
  for(const p of ['/api/credentials','/api/toolhub/assets','/api/resources','/api/runs'])assert.equal((await request(p)).status,404);
  assert.equal(requests.length,count,'business BFF must not forward administration APIs');
 }
 for(let i=0;i<100&&!instanceSeen;i++)await sleep(50);
 assert.ok(instanceSeen,'actual current process heartbeat');if(fixtureError)throw fixtureError;
 success=true;
}finally{
 let stopped,cleanupError;
 try{
  if(child.pid&&child.exitCode===null&&!child.signalCode&&!child.kill())cleanupError=Error('fixture process termination failed');
  stopped=await Promise.race([exited,sleep(5000).then(()=>null)]);
  if(!stopped)cleanupError=Error('fixture process did not exit');
 }finally{
  fs.closeSync(log);upstream.closeAllConnections();await new Promise(resolve=>upstream.close(resolve));
  fs.writeFileSync(path.join(fixture,'result.json'),JSON.stringify({passed:success&&!cleanupError,role,commit:manifest.commit,scope:'actual relocated BFF and Web; synthetic authenticated HTTP peers; no database or model',requests:requests.length,heartbeat:instanceSeen,process_stopped:Boolean(stopped)},null,2));
 }
 if(cleanupError)throw cleanupError;
}
console.log(JSON.stringify({passed:true,role,fixture,scope:'real executable and Web; synthetic downstream peers'}));
