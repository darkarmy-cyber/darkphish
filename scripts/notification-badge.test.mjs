import test from "node:test";
import assert from "node:assert/strict";
import {readFileSync} from "node:fs";
import vm from "node:vm";

test("badge counts a stable update and renders release text without HTML",()=>{
 const elements=new Map();
 const $=selector=>{
  if(typeof selector==="function")return;
  if(!elements.has(selector))elements.set(selector,{text(v){this.value=v;return this},prop(k,v){this[k]=v;return this}});
  return elements.get(selector);
 };
 const context=vm.createContext({$});vm.runInContext(readFileSync(new URL("../static/js/src/app/notifications.js",import.meta.url),"utf8"),context);
 context.renderAdminNotification({available:true,badge:1,latest_version:"<img onerror=alert(1)>"});
 assert.equal(elements.get("#adminNotificationBadge").value,1);assert.equal(elements.get("#adminNotificationBadge").hidden,false);
 assert.equal(elements.get("#adminReleaseNotification").value,"Darkphish <img onerror=alert(1)> is available");
 context.renderAdminNotification({available:false,badge:1});assert.equal(elements.get("#adminNotificationBadge").hidden,true);
 context.renderAdminNotification({available:true,badge:99});assert.equal(elements.get("#adminNotificationBadge").hidden,true);
});
