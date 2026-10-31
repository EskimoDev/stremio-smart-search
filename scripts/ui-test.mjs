import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import vm from 'node:vm';

const source = await readFile(new URL('../internal/app/web/app.js', import.meta.url), 'utf8');
const ids = ['manifest','message','connection','build','progress','stage','catalog-info','web-install','launch','version','copy','quit','install-help','install-stremio','install-dxbi','install-enhanced'];
const nodes = Object.fromEntries(ids.map(id => [id, { value:'', disabled:false, dataset:{}, classList:{toggle(){}}, setAttribute(){}, removeAttribute(name){delete this[name];}, addEventListener(event,callback){this[event]=callback;}, focus(){}, select(){this.selected=true;} }]));
const buttons = ['stremio','dxbi','enhanced'].map(id => { const node=nodes[`install-${id}`];node.dataset.client=id;return node; });
const manifest = 'http://127.0.0.1:11479/manifest.json';
const webInstall = `https://web.stremio.com/#/addons?addon=${encodeURIComponent(manifest)}`;
let copied, clipboardBlocked=false;
const requests=[];
const context = {
  document:{getElementById:id=>nodes[id],querySelectorAll:()=>buttons},
  window:{smartSearch:{baseURL:'http://127.0.0.1:11479',token:'fixture'}},
  navigator:{clipboard:{async writeText(value){if(clipboardBlocked)throw Error('blocked');copied=value;}}},
  setTimeout(){},clearTimeout(){},
  async fetch(path,options){requests.push({path,options});const value = path==='/api/status' ? {ready:true,movies:1,series:1,builtAt:'2026-10-31',manifest,webInstall,preferredClient:'system',product:'Smart Search',version:'test'} : path==='/api/clients' ? {clients:buttons.map(node=>({id:node.dataset.client,name:node.dataset.client,available:true}))} : {name:'dx.bi'};return {ok:true,async json(){return value;}};},
};
vm.runInNewContext(source,context);
await new Promise(resolve=>setImmediate(resolve));
assert.equal(nodes['web-install'].href,webInstall);
assert.equal(new URLSearchParams(new URL(nodes['web-install'].href).hash.split('?')[1]).get('addon'),manifest);
assert(buttons.every(button=>!button.disabled));
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
console.log('Setup UI checks passed: HTTP transport, client choice, clipboard and fallback.');
