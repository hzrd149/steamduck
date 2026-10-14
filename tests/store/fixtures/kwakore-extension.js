// Test fixture: Kwakore extension helper with 180-second native budgets.

const extensionRoot = document.documentElement;
const extensionChannel = new MessageChannel();
const extensionPending = new Map();
let extensionSerial = 0;
const extensionCurrent = () => document.documentElement === extensionRoot;
const extensionEnded = () => {
 extensionChannel.port1.postMessage({type:'__kwakore.extension.replaced'});
 for (const entry of extensionPending.values()) {
  clearTimeout(entry.timer);
  entry.reject({code:-32000,message:'Custom document ended',reason:'document-ended'});
 }
 extensionPending.clear();
};
const extensionObserver = new MutationObserver(() => {
 if (!extensionCurrent()) { extensionObserver.disconnect(); extensionEnded(); }
});
extensionObserver.observe(document,{childList:true});
extensionChannel.port1.onmessage = event => {
 if (!extensionCurrent()) { extensionEnded(); return; }
 const env = event.data;
 if (!env || typeof env.type !== 'string') return;
 const entry = extensionPending.get(env.id);
 if (!entry || env.type !== entry.type + '.result') return;
 extensionPending.delete(env.id); clearTimeout(entry.timer);
 if (env.error) entry.reject(env.error); else entry.resolve(env.result);
};
const napExtension = Object.freeze({call(type, fields, deadlineMS = 185000) {
 if (!extensionCurrent()) { extensionEnded(); return Promise.reject({code:-32000,message:'Custom document ended',reason:'document-ended'}); }
 if (extensionPending.size >= 64) return Promise.reject({code:-32003,message:'Custom request failed',reason:'rate-limited'});
 return new Promise((resolve,reject) => {
  const id = 'custom-' + (++extensionSerial);
  const timer = setTimeout(() => {
   extensionPending.delete(id);
   reject({code:-32004,message:'Custom request failed',reason:'timeout'});
  }, Math.min(185000,Math.max(1,deadlineMS)));
  extensionPending.set(id,{type,resolve,reject,timer});
  try { extensionChannel.port1.postMessage({...fields,type,id}); }
  catch(error) { extensionPending.delete(id); clearTimeout(timer); reject({code:-32001,message:'Custom request failed',reason:'invalid-request'}); }
 });
}});
