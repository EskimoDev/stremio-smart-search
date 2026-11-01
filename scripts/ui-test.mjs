import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import vm from 'node:vm';

const source = await readFile(new URL('../internal/app/web/app.js', import.meta.url), 'utf8');
const ids = ['manifest','message','connection','build','progress','stage','catalog-info','catalog-stats','movie-count','series-count','catalog-profile','profile-info','web-install','launch','version','copy','quit','install-help','install-stremio','install-dxbi','install-enhanced','client-status-stremio','client-status-dxbi','client-status-enhanced'];
const nodes = Object.fromEntries(ids.map(id => [id, { value:'', disabled:false, dataset:{}, classList:{toggle(){}}, setAttribute(){}, removeAttribute(name){delete this[name];}, addEventListener(event,callback){this[event]=callback;}, focus(){}, select(){this.selected=true;} }]));
const buttons = ['stremio','dxbi','enhanced'].map(id => { const node=nodes[`install-${id}`];node.dataset.client=id;return node; });
const manifest = 'http://127.0.0.1:11479/manifest.json';
const webInstall = `https://web.stremio.com/#/addons?addon=${encodeURIComponent(manifest)}`;
let copied, clipboardBlocked=false;
let nextRefresh;
let status = {ready:true,building:false,profile:'compact',profiles:[{id:'compact',name:'Compact',movies:25000,series:25000,minVotes:250},{id:'standard',name:'Standard',movies:100000,series:100000,minVotes:25},{id:'expanded',name:'Expanded',movies:250000,series:250000,minVotes:5}],movies:25000,series:12500,builtAt:'2026-10-31',manifest,webInstall,preferredClient:'system',product:'Smart Search',version:'test'};
const requests=[];
const context = {
  document:{getElementById:id=>nodes[id],querySelectorAll:()=>buttons},
  window:{smartSearch:{baseURL:'http://127.0.0.1:11479',token:'fixture'}},
  navigator:{clipboard:{async writeText(value){if(clipboardBlocked)throw Error('blocked');copied=value;}}},
  setTimeout(callback){nextRefresh=callback;},clearTimeout(){},
  async fetch(path,options){requests.push({path,options});if(path==='/api/build')status={...status,building:true,buildProfile:JSON.parse(options.body).profile};const value = path==='/api/status'||path==='/api/build' ? status : path==='/api/clients' ? {clients:buttons.map(node=>({id:node.dataset.client,name:node.dataset.client,available:true}))} : {name:'dx.bi'};return {ok:true,async json(){return value;}};},
};
vm.runInNewContext(source,context);
await new Promise(resolve=>setImmediate(resolve));
assert.equal(nodes['web-install'].href,webInstall);
assert.equal(new URLSearchParams(new URL(nodes['web-install'].href).hash.split('?')[1]).get('addon'),manifest);
assert(buttons.every(button=>!button.disabled));
assert.equal(nodes['catalog-stats'].hidden,false);
assert.equal(nodes['movie-count'].textContent,(25000).toLocaleString());
assert.equal(nodes['series-count'].textContent,(12500).toLocaleString());
assert.match(nodes['client-status-stremio'].textContent,/Detected on this computer/);
await nodes['install-dxbi'].click();
assert.equal(copied,manifest);
assert.deepEqual(JSON.parse(requests.find(request=>request.path==='/api/install').options.body),{client:'dxbi'});
assert.match(nodes['install-help'].textContent,/dx\.bi/);
assert.match(nodes.launch.textContent,/dx\.bi/);
const launches=requests.filter(request=>request.path==='/api/install').length;
clipboardBlocked=true;
await nodes['install-enhanced'].click();
assert.equal(requests.filter(request=>request.path==='/api/install').length,launches);
assert(nodes.manifest.selected);
nodes['catalog-profile'].value='expanded';
nodes['catalog-profile'].change();
assert.match(nodes.build.textContent,/Apply size/);
await nextRefresh();
assert.equal(nodes['catalog-profile'].value,'expanded','polling must preserve an unapplied selection');
await nodes.build.click();
await new Promise(resolve=>setImmediate(resolve));
assert.equal(JSON.parse(requests.find(request=>request.path==='/api/build').options.body).profile,'expanded');
assert.equal(nodes['catalog-profile'].disabled,true);
assert.equal(nodes['catalog-stats'].hidden,false,'keep existing catalogue visible during refresh');
assert(buttons.every(button=>!button.disabled),'working catalogue remains installable during refresh');
status={...status,building:false,buildProfile:'',error:'Download failed'};
await nextRefresh();
assert.equal(nodes['catalog-profile'].value,'expanded');
assert.match(nodes['catalog-info'].textContent,/Compact/);
assert.match(nodes.message.textContent,/previous catalogue is still available/);
status={...status,error:'',building:true,buildProfile:'expanded'};
await nextRefresh();
status={...status,building:false,profile:'expanded',buildProfile:''};
await nextRefresh();
assert.match(nodes['catalog-info'].textContent,/Expanded/);
assert.equal(nodes.build.textContent,'Refresh catalogue');
status={...status,ready:false,movies:0,series:0};
await nextRefresh();
assert.equal(nodes['catalog-stats'].hidden,true);
assert(buttons.every(button=>button.disabled));
assert.match(nodes['client-status-stremio'].textContent,/prepare catalogue first/);
assert.equal(nodes['web-install'].href,undefined);
assert.equal(nodes.build.disabled,false);
status={...status,building:true,stage:'Downloading title data',bytes:2*1024*1024};
await nextRefresh();
assert.equal(nodes.progress.hidden,false);
assert.equal(nodes.build.disabled,true);
assert.match(nodes.stage.textContent,/2.0 MB downloaded/);
assert(buttons.every(button=>button.disabled));
console.log('Setup UI checks passed: HTTP install, clipboard fallback, profile selection, refresh and failure states.');
