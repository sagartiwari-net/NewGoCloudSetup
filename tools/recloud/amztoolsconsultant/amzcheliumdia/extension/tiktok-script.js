(()=>{var e,t,n,r={56639:(e,t,n)=>{"use strict";n.d(t,{d:()=>r,z:()=>o});const r={ON_LOGIN:"login",CHECK_LANGUAGE:"check-language",CHECK_SHOPIFY:"check-shopify",NEW_INCOGNITO_TAB:"new-incognito-tab",OPEN_OPTIONS:"open-options",OPEN_PAGE:"open-page",OPEN_PAGE_XRAY:"open-page-and-xray",OPEN_PAGE_WALMART_XRAY:"open-page-and-walmart-xray",RELOAD_TABS:"reload-tabs",REVIEW_REQUEST:"review-request",BACKGROUND_REQUEST:"background-request",PRODUCT_REQUEST:"product-request",SEGMENT_EVENT:"segment-event",DOWNLOAD:"download",STOCK_REQUEST:"stock-request",COMMON_REQUEST:"common-request",GET_GLOBAL_CONFIG:"get-global-config",GET_GLOBAL_CONFIG_GRAYSCALE_DATE_RANGE:"get-global-config-grayscale-date-range",GET_INCOGNITO_STATE:"get-incognito-state"},o={OPEN_ASIN_GRABBER:"h10-asin-grabber",OPEN_CALCULATOR:"h10-profitability-calculator",OPEN_CALCULATOR_WALMART:"h10-profitability-calculator-walmart",OPEN_INVENTORY:"h10-inventory",OPEN_REVIEW_INSIGHTS:"h10-review",OPEN_XRAY:"h10-xray",OPEN_XRAY_WALMART:"h10-xray-walmart",OPEN_XRAY_KEYWORDS:"h10-xray-keywords",AMAZON_PRODUCT:"on-amazon-product-page",WALMART_PRODUCT:"on-walmart-product-page",TIKTOK_PRODUCT:"on-tiktok-product-page",GET_ASIN_ORIGINS:"get-asin-origins",IS_FILES_INJECTED:"is-files-injected",OPEN_AI_REVIEW_INSIGHTS:"h10-ai-review-insights",OPEN_CATEGORY_REVIEW_RETURNS:"h10-category-review-returns",OPEN_TIKTOK_REVIEW_INSIGHTS:"h10-tiktok-review-insights",OPEN_UPSELL_MODAL:"h10-open-upsell-modal",GET_CATEGORY_REVIEW_RETURNS_AVAILABLE:"get-category-review-returns-available"}},18780:(e,t,n)=>{"use strict";n.d(t,{BF:()=>g,Pn:()=>l,Tr:()=>m,WL:()=>d,jy:()=>h,uc:()=>p});var r=n(56639),o=n(59339),a=Math.pow,i=(e,t,n)=>new Promise(((r,o)=>{var a=e=>{try{l(n.next(e))}catch(e){o(e)}},i=e=>{try{l(n.throw(e))}catch(e){o(e)}},l=e=>e.done?r(e.value):Promise.resolve(e.value).then(a,i);l((n=n.apply(e,t)).next())}));const l="incognito_";let s=!1,u=!1;const c=()=>{var e;return"boolean"==typeof(null==(e=null==chrome?void 0:chrome.extension)?void 0:e.inIncognitoContext)?chrome.extension.inIncognitoContext:null},d=()=>i(void 0,null,(function*(){if(u)return;const e=c();if(null!==e)return s=e,u=!0,void(o.o&&console.log("[INCOGNITO] Content-script detection (inIncognitoContext):",s?"INCOGNITO":"NORMAL"));const t=yield((e=4,t=200)=>i(void 0,null,(function*(){for(let n=0;n<e;n++){if(n>0){const e=t*a(2,n-1);yield new Promise((t=>setTimeout(t,e)))}try{return!!(yield chrome.runtime.sendMessage({type:r.d.GET_INCOGNITO_STATE}))}catch(t){o.o&&console.log(`[INCOGNITO] sendMessage attempt ${n+1}/${e} failed:`,t)}}return null})))();if(null!==t)return s=t,u=!0,void(o.o&&console.log("[INCOGNITO] Content-script detection (background message):",s?"INCOGNITO":"NORMAL"));o.o&&console.log("[INCOGNITO] Content-script detection failed after all retries \u2014 failing closed (assuming INCOGNITO)"),s=!0,u=!0})),p=e=>{if(!u){const e=c();null!==e&&(s=e,u=!0)}return s?e.startsWith(l)?e:`${l}${e}`:e},g=e=>e.map((e=>p(e))),h=(e,t)=>{var n;const r={};for(const o of t){const t=p(o);r[o]=null!=(n=e[t])?n:null}return r},m=e=>{const t={};for(const[n,r]of Object.entries(e)){t[p(n)]=r}return t}},89697:(e,t,n)=>{"use strict";n.d(t,{B1:()=>O.B,ID:()=>T,nE:()=>I,BF:()=>b,WL:()=>r.WL,OZ:()=>C,xl:()=>M,w2:()=>L,ye:()=>s,Gj:()=>u,ZF:()=>S,PQ:()=>l,Ah:()=>j,ow:()=>c,o7:()=>o.o});var r=n(18780),o=n(59339);const a=e=>chrome.storage[["cache-dynamic","cache-local","local"].includes(e)?"local":"sync"],i=(e,t)=>{o.o&&console.log("STORE - SET",e,t);const n=(0,r.Tr)(t);return a(e).set(n)},l=(e,t)=>i(e,t),s=(e,t,n,i={})=>{const l=a(e),s=Array.isArray(t)?t:[t],u=p(e,s,n);return o.o&&console.log(`STORE - SUBSCRIBE!, ${e}, [${s.join(", ")}]`),l.onChanged.addListener(u),i.ignoreInitial||setTimeout((()=>l.get((0,r.BF)(s)).then((e=>{const t=(0,r.jy)(e,s);d(s,n)(t)})))),()=>{o.o&&console.log(`STORE - UNSUBSCRIBE!, ${e}, [${s.join(", ")}]`),l.onChanged.removeListener(u)}},u=(e,t)=>new Promise((n=>{const i=Array.isArray(t)?t:[t];setTimeout((()=>{o.o&&console.log(`STORE - ONCE!, ${e}, [${i.join(", ")}]`),a(e).get((0,r.BF)(i)).then((e=>{const t=(0,r.jy)(e,i);n(t)}))}))})),c=(e,t,n)=>new Promise((r=>{u(e,t).then((({[t]:o})=>{l(e,{[t]:n(o)}).then(r)}))})),d=(e,t)=>n=>{const r=e.reduce(((e,t)=>(e[t]=void 0!==n[t]?n[t]:null,e)),{});o.B&&console.log(`STORE - INITIAL DATA, [${e.join(", ")}]`,JSON.parse(JSON.stringify(r))),t(r)},p=(e,t,n)=>i=>{const l=Object.entries(i).filter((([e])=>{const n=e.startsWith(r.Pn)?e.replace(r.Pn,""):e;return t.includes(n)}));o.B&&console.log(`STORE - LISTENER, [${t.join(", ")}]`,JSON.parse(JSON.stringify(i)),JSON.parse(JSON.stringify(l))),l.length&&a(e).get((0,r.BF)(t)).then((e=>{const a=l.reduce(((e,[t,n])=>(e[t.startsWith(r.Pn)?t.replace(r.Pn,""):t]=void 0!==n.newValue?n.newValue:null,e)),{});t.forEach((t=>{var n;if(void 0===a[t]){const o=String(t);a[t]=null!=(n=e[(0,r.uc)(o)])?n:null}})),o.o&&console.log(`STORE - LISTENER DATA, [${t.join(", ")}]`,JSON.parse(JSON.stringify(a))),n(a)}))};var g=Object.defineProperty,h=Object.defineProperties,m=Object.getOwnPropertyDescriptors,f=Object.getOwnPropertySymbols,v=Object.prototype.hasOwnProperty,y=Object.prototype.propertyIsEnumerable,k=(e,t,n)=>t in e?g(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n,x=(e,t,n)=>new Promise(((r,o)=>{var a=e=>{try{l(n.next(e))}catch(e){o(e)}},i=e=>{try{l(n.throw(e))}catch(e){o(e)}},l=e=>e.done?r(e.value):Promise.resolve(e.value).then(a,i);l((n=n.apply(e,t)).next())}));const w=e=>(null==e?void 0:e.ready)&&(null==e?void 0:e.expirationTime)&&new Date(e.expirationTime)>new Date&&e.data||null,b=(e=6)=>{const t=new Date;return t.setHours(t.getHours()+e)},C=(e,t,n)=>s(e,t,(e=>n(E(e)))),M=(e,t)=>u(e,t).then((e=>E(e))),L=(e,t)=>u("cache-dynamic",e).then((({[e]:n})=>D(n,t))),S=(e,t,n,r)=>x(void 0,null,(function*(){return o.o&&console.log(`CACHE - SET, ${e}: "${String(t)}"`,n),i(e,{[t]:{data:n,expirationTime:b(r),ready:!!n}})})),j=(e,t,n,r)=>x(void 0,null,(function*(){u("cache-dynamic",e).then((({[e]:o})=>{const a=(l=((e,t)=>{for(var n in t||(t={}))v.call(t,n)&&k(e,n,t[n]);if(f)for(var n of f(t))y.call(t,n)&&k(e,n,t[n]);return e})({},o),s={[t]:{data:n,expirationTime:b(r),ready:!!n}},h(l,m(s)));var l,s;i("cache-dynamic",{[e]:a})}))})),I=(e,t,n,r)=>{u(e,t).then((({[t]:a})=>{const i=w(a);o.o&&console.log(`CACHE - UPDATE! ${e}: "${String(t)}"`,JSON.parse(JSON.stringify(a))),i||P(e,t,n,r)}))},T=(e,t)=>x(void 0,null,(function*(){const n=Array.isArray(t)?t:[t];return o.o&&console.log(`CACHE - CLEAR, [${n.join(", ")}]`),i(e,n.reduce(((e,t)=>(e[t]={ready:!1},e)),{}))})),P=(e,t,n,r)=>{o.o&&console.log(`CACHE - FETCH, "${String(t)}"`),n().then((n=>S(e,t,n))).catch((e=>{o.o&&console.log("CACHE - FETCH ERROR",e),null==r||r(e)}))},E=e=>Object.entries(e).reduce(((e,[t,n])=>(o.o&&console.log(`CACHE - GET! "${t}"`,n),e[t]=w(n),e)),{}),D=(e,t)=>(o.o&&console.log(`CACHE - GET! "${t}"`,null==e?void 0:e[t]),(null==e?void 0:e[t])?w(e[t]):null);var O=n(32166)},59339:(e,t,n)=>{"use strict";n.d(t,{B:()=>o,o:()=>r});const r=!1,o=r&&!1},32166:(e,t,n)=>{"use strict";n.d(t,{B:()=>r});const r="CORE_BEARER_TOKEN"},53738:(e,t,n)=>{"use strict";n.d(t,{nU:()=>a});var r,o;null==(o=((null==(r=null==chrome?void 0:chrome.runtime)?void 0:r.getManifest())||{}).content_scripts)||o.flatMap((e=>e.matches||[])).map((e=>e||"")).filter((e=>"*://*/*"!==e)).map((e=>null==e?void 0:e.replace(/\./g,"\\."))).map((e=>null==e?void 0:e.replace(/\*/g,".*"))).map((e=>new RegExp(e,"")));const a=()=>new Promise((e=>{chrome.tabs?chrome.tabs.query({currentWindow:!0,active:!0},(t=>e(t[0]))):e(void 0)}))},91255:(e,t,n)=>{"use strict";n.d(t,{F:()=>r});const r={"open-black-box":"xray","open-xray-walmart":"xrayWalmart","open-xray-keywords":"xrayKeywords","open-asin-grabber":"asinGrabber","open-calculator":"profitCalc","open-calculator-walmart":"profitCalcWalmart","open-inventory":"inventoryLevels","open-review-insights":"reviewInsights","open-ai-review-insights":"AmazonReviewInsightsNew","open-category-review-returns":"AmazonNicheReviewInsightsNew","open-analyzer":"demandAnalyzer",salesEstimator:"salesEstimator",supplierFinder:"supplierFinder",serpToolbar:"serpToolbar",bsrWidget:"bsrWidget",influencer_download_video:"influencer_download_video","open-tiktok-review-insights":"TikTokReviewInsights"}},9393:(e,t,n)=>{"use strict";n.d(t,{o:()=>o});var r=n(89697);const o=e=>(0,r.Gj)("sync","accountId").then((({accountId:t})=>{const n=new URL(e);return t&&-1!==t&&n.searchParams.append("accountId",`${t}`),n.toString()}))},73212:(e,t,n)=>{"use strict";n(89697),n(31810),Object.defineProperty,Object.defineProperties,Object.getOwnPropertyDescriptors,Object.getOwnPropertySymbols,Object.prototype.hasOwnProperty,Object.prototype.propertyIsEnumerable},74938:(e,t,n)=>{"use strict";n.d(t,{p:()=>o});const r=new Set(["stripbooks","digital-text","audible","books"]),o=()=>{try{if(!a())return!1;if((()=>{const e=new URLSearchParams(window.location.search),t=window.location.pathname,n=["i","s"].map((t=>{var n;return null==(n=e.get(t))?void 0:n.toLowerCase()})).filter(Boolean),r=(()=>{const e=t.match(/\/zgbs\/([^/]+)/i)||t.match(/\/gp\/(bestsellers|new-releases|movers-and-shakers|most-wished-for|most-gifted)\/([^/]+)/i),n=(null==e?void 0:e[2])||(null==e?void 0:e[1]);return n?[n.toLowerCase()]:[]})();return Array.from(new Set([...n,...r]))})().some((e=>r.has(e))))return!0;if((e=>{const t=(null==e?void 0:e.toLowerCase())||"";return t.includes("book")||t.includes("kindle")})((()=>{var e;const t=document.querySelector("#wayfinding-breadcrumbs_feature_div ul li a");return null==(e=null==t?void 0:t.textContent)?void 0:e.trim()})()))return!0;if((()=>{var e;const t=document.querySelector("#nav-progressive-subnav");if(!t)return!1;const n=t.querySelector('a[href^="/your-books"]'),r=null==(e=null==n?void 0:n.textContent)?void 0:e.trim();return"Your Books"===r||"Your Audiobooks"===r})())return!0}catch(e){console.warn("[isKDPPage] failed:",e)}return!1},a=()=>{try{const e=new URL(window.location.href);return"www.amazon.com"===e.hostname||"amazon.com"===e.hostname}catch(e){return console.warn("[isAmazonComPage] failed to parse URL:",e),!1}}},31810:(e,t,n)=>{"use strict";n.d(t,{B:()=>a});var r=n(27552),o=n(6892);const a=e=>{return t=void 0,n=null,a=function*(){const t=(0,r.realTokenLink)(e);return fetch(t,{}).then((e=>(0,o.L)(e)))},new Promise(((e,r)=>{var o=e=>{try{l(a.next(e))}catch(e){r(e)}},i=e=>{try{l(a.throw(e))}catch(e){r(e)}},l=t=>t.done?e(t.value):Promise.resolve(t.value).then(o,i);l((a=a.apply(t,n)).next())}));var t,n,a}},6892:(e,t,n)=>{"use strict";n.d(t,{L:()=>r,R:()=>o});const r=(e,...t)=>{return n=void 0,r=[e,...t],o=function*(e,t={}){return 200===e.status?yield e.json():t},new Promise(((e,t)=>{var a=e=>{try{l(o.next(e))}catch(e){t(e)}},i=e=>{try{l(o.throw(e))}catch(e){t(e)}},l=t=>t.done?e(t.value):Promise.resolve(t.value).then(a,i);l((o=o.apply(n,r)).next())}));var n,r,o},o=e=>{var t;if(!e||!e.data)return null;const n=e.data,r={isChinese:n.isChinese,plan:{key:null==(t=n.plan)?void 0:t.key},features:{}};return n.features&&Object.keys(n.features).forEach((e=>{const t=n.features[e];r.features[e]={hasAccess:null==t?void 0:t.hasAccess,hasFullAccess:null==t?void 0:t.hasFullAccess}})),r}},47508:(e,t,n)=>{"use strict";n.d(t,{F$:()=>h,FT:()=>m,U7:()=>a,a$:()=>c,lm:()=>y,p5:()=>g,pz:()=>p,xy:()=>v});var r=n(89697),o=(e,t,n)=>new Promise(((r,o)=>{var a=e=>{try{l(n.next(e))}catch(e){o(e)}},i=e=>{try{l(n.throw(e))}catch(e){o(e)}},l=e=>e.done?r(e.value):Promise.resolve(e.value).then(a,i);l((n=n.apply(e,t)).next())}));const a={language:"h10-amazon-language",entitlements:"h10-entitlements",parserConfig:"h10-parser-config",currencyList:"h10-currency-list",usesLeft:"h10-uses-left",asinRequested:"h10-asin-requested-",asinParsed:"h10-asin-parsed-",sellerCountryParsed:"h10-amazon-seller-country-"},i=e=>!!e&&new Date(e)>new Date,l=e=>new Promise((t=>{var n,r;try{null==(r=null==(n=null==chrome?void 0:chrome.storage)?void 0:n.local)||r.get([e],(n=>{t(n[e])}))}catch(e){t(null)}})),s=(e,t,n)=>{var o,a;const i=(0,r.BF)(n);null==(a=null==(o=null==chrome?void 0:chrome.storage)?void 0:o.local)||a.set({[e]:{data:t,expirationTime:i,ready:!0}})},u=(e,t)=>o(void 0,null,(function*(){return t?i(null==t?void 0:t.expirationTime)&&t.data||null:i(null==e?void 0:e.expirationTime)&&(null==e?void 0:e.data)||null})),c=(e,t,n)=>o(void 0,null,(function*(){const r=yield d(t+e);return u(r,n)})),d=e=>l(e),p=e=>new Promise((t=>{var n,r;try{null==(r=null==(n=null==chrome?void 0:chrome.storage)?void 0:n.local)||r.remove([e],(()=>{t(null)}))}catch(e){t(null)}})),g=(e,t,n)=>o(void 0,null,(function*(){const r=yield f(e,t);return u(r,n)})),h=(e,t)=>s(a.sellerCountryParsed+e,t,48),m=(e,t,n)=>s(`${a.asinParsed}${t}-`+e,n),f=(e,t)=>l(`${a.asinRequested}${e}-${t}`),v=(e,t,n)=>s(`${a.asinRequested}${e}-${t}`,n),y=e=>new Promise((t=>{var n,r;try{null==(r=null==(n=null==chrome?void 0:chrome.storage)?void 0:n.local)||r.remove([e],(()=>{t(null)}))}catch(e){t(null)}}))},98150:(e,t,n)=>{"use strict";n.d(t,{FP:()=>w,KK:()=>k,Py:()=>y,hl:()=>x});var r=n(56639),o=n(18780),a=n(53738),i=n(57677),l=n(74938),s=n(10256),u=Object.defineProperty,c=Object.defineProperties,d=Object.getOwnPropertyDescriptors,p=Object.getOwnPropertySymbols,g=Object.prototype.hasOwnProperty,h=Object.prototype.propertyIsEnumerable,m=(e,t,n)=>t in e?u(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n,f=(e,t)=>{for(var n in t||(t={}))g.call(t,n)&&m(e,n,t[n]);if(p)for(var n of p(t))h.call(t,n)&&m(e,n,t[n]);return e},v=(e,t,n)=>new Promise(((r,o)=>{var a=e=>{try{l(n.next(e))}catch(e){o(e)}},i=e=>{try{l(n.throw(e))}catch(e){o(e)}},l=e=>e.done?r(e.value):Promise.resolve(e.value).then(a,i);l((n=n.apply(e,t)).next())}));const y=(e,t={},n={})=>chrome.runtime.sendMessage(f({type:e,params:t},n)),k=(e,t={})=>(0,a.nU)().then((n=>((e,t,...n)=>v(void 0,[e,t,...n],(function*(e,t,n={}){return new Promise(((r,o)=>{(null==e?void 0:e.id)?chrome.tabs.sendMessage(e.id,{type:t,params:n},(e=>r(e))):o("Tab does not exist")}))})))(n,e,t))),x=e=>e.fromBackground?(0,i.b)(e):chrome.runtime.sendMessage({type:r.d.ON_LOGIN,params:e}),w=e=>v(void 0,null,(function*(){var t;const n=(0,o.uc)("mode"),a="seller"===(yield chrome.storage.sync.get(n))[n];if(!s.zP.includes(e.name)&&a){const n=(0,l.p)()?"KDP":"Physical_Product";e.properties=(i=f({},e.properties||{}),u={product_type:(null==(t=e.properties)?void 0:t.product_type)||n},c(i,d(u)))}var i,u;return e.fromBackground?(0,s.M2)(e):chrome.runtime.sendMessage({type:r.d.SEGMENT_EVENT,params:e})}))},10256:(e,t,n)=>{"use strict";n.d(t,{M2:()=>g,ny:()=>d,zP:()=>p});var r=n(89697),o=n(95633),a=Object.defineProperty,i=Object.getOwnPropertySymbols,l=Object.prototype.hasOwnProperty,s=Object.prototype.propertyIsEnumerable,u=(e,t,n)=>t in e?a(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n,c=(e,t)=>{for(var n in t||(t={}))l.call(t,n)&&u(e,n,t[n]);if(i)for(var n of i(t))s.call(t,n)&&u(e,n,t[n]);return e};const d={feedback:"Extension Feedback",widgetButtonClicked:"Extension Widget Button Clicked",languageSwitched:"Extension Language Switched",listingAnalyzerClicked:"Extension Listing Analyzer Button Clicked",toolLaunched:"Extension Tool Launched",reviewsExtracted:"Extension Review Insights Reviews Extracted",upsellModalFired:"Upsell Modal Fired",alertsEngaged:"Extension Alerts Engaged",keywordActionTaken:"Xray Keyword Action Taken",searchExpanderActionTaken:"Search Expander Action Taken",toolbarEngaged:"Extension Toolbar Engaged",onboardingStepCompleted:"Extension Onboarding Step Completed",competitorAddedFrom:"Competitor Added From",serpToolbarEngaged:"SERP Toolbar Engaged",serpToolbarLaunched:"SERP Toolbar Launched",changeLogClicked:"Extension View Change Log Clicked",xrayHeaderCTAClicked:"HCE Xray Header CTA Clicked",searchExpanderLaunched:"Free User Search Expander Launched",productIdeaCreate:"PID_Create",pdpToolbarCtaClicked:"PDP Toolbar CTA Clicked",sizeTierOptimizationToggleEngaged:"Extension Size Optimizer Toggle Engaged",pcToggleEngaged:"HCE PC Toggle Engaged",modeSwitcher:"User Mode Switcher",influencerModeVideoTypeRecord:"Influencer Mode Video Type Record",findSimilar:"Find Similar",ebayPriceChecker:"Ebay Price Checker",HCEComponentLoadTime:"HCE Component Load Time",keywordTranslationClick:"Keyword Translation Click",purchaseHistoryScannerXrayOpened:"Analyze Products Clicked",purchaseHistoryScannerLoad30DaySales:"Load-30-day Sales Data Clicked",creatorStudioVideoDownload:"Download Clicked",creatorConnectionWidgetTurnedOn:"Creator Connection Widget Turned On",aiReviewInsightsEngagement:"Amazon Review Insights Engagement",tiktokReviewInsightsEngagement:"TikTok Review Insights Engagement",keywordTrackerEngagement:"HCE Keyword Tracker Engagement"},p=["Extension Feedback","Extension Language Switched","User Mode Switcher","Influencer Mode Video Type Record","HCE Component Load Time","Analyze Products Clicked","Load-30-day Sales Data Clicked","Download Clicked","Extension Alerts Engaged","Extension View Change Log Clicked"],g=e=>{return t=void 0,n=null,a=function*(){try{console.log("Segment Event!",e);const t=yield(0,r.Gj)("sync","settings").then((({settings:e})=>(null==e?void 0:e.language)||"en"));return fetch(o.iH,{method:"POST",headers:{Accept:"application/json","Content-Type":"application/json"},body:JSON.stringify({name:e.name,properties:c({language:t,marketplace:e.marketplace},e.properties)})})}catch(e){console.log("Segment Event Failed!",e)}},new Promise(((e,r)=>{var o=e=>{try{l(a.next(e))}catch(e){r(e)}},i=e=>{try{l(a.throw(e))}catch(e){r(e)}},l=t=>t.done?e(t.value):Promise.resolve(t.value).then(o,i);l((a=a.apply(t,n)).next())}));var t,n,a}},27552:(e,t,n)=>{"use strict";n.d(t,{TOKEN_KEY:()=>o.B1,abaDataLink:()=>L.fn,amazonLink:()=>C.P2,amazonUserSettingLink:()=>C.gz,backgroundMessageTypes:()=>r.d,bsrGraphDataLink:()=>L.sI,cachePrefixes:()=>s.U7,clearCachedData:()=>o.ID,controlCenterLink:()=>M.R1,feedbackFormLink:()=>M.D3,feedbackSurvey:()=>M.GZ,fetchCacheDataIfNeeded:()=>o.nE,getBSRDataCacheKey:()=>p.B3,getCheckLoginLink:()=>L.Z0,getCountryLink:()=>L.JY,getCoupon:()=>L.DE,getCurrentTab:()=>a.nU,getExistParsed:()=>s.a$,getExistSales:()=>s.p5,getExpirationDate:()=>o.BF,getTiktokReviewChargeUseLink:()=>L.nJ,getUpgradeWithCouponPlanLinkV1:()=>L.I6,getUpgradeWithCouponPlanLinkV2:()=>L.Bx,heliumExtensionInStore:()=>M.XN,initIncognitoForContentScript:()=>o.WL,learnHowToUseLink:()=>M.Ko,mainHeliumLink:()=>M.XP,mainTools:()=>i.F,membersLink:()=>L.Cn,onGetCacheData:()=>o.OZ,onGetCacheDataOnce:()=>o.xl,onGetDynamicCacheDataOnce:()=>o.w2,onGetStore:()=>o.ye,onGetStoreOnce:()=>o.Gj,parserConfigLink:()=>L.j1,realTokenLink:()=>L.Bh,reduceAccountInfo:()=>g.e,reduceCheckLogin:()=>f.w,removeCachedAsin:()=>s.lm,removeCachedEntity:()=>s.pz,researchLink:()=>M.HF,reviewGraphDataLink:()=>L.hc,salesGraphDataLink:()=>L.Z1,salesLink:()=>L.t9,searchGoogleGraphDataLink:()=>M.r8,segmentEventType:()=>c.ny,sendBackgroundLoginMessage:()=>u.hl,sendBackgroundMessage:()=>u.Py,sendCurrentTabMessage:()=>u.KK,sendSegmentEventMessage:()=>u.FP,setCountryCache:()=>s.F$,setParsedCache:()=>s.FT,setSalesCache:()=>s.xy,setStaticCachedData:()=>o.ZF,setStore:()=>o.PQ,signupLink:()=>L.j_,tabMessageTypes:()=>r.z,tokenConnectionLink:()=>L.mS,transformIntoCNLink:()=>L.OG,updateDynamicCachedData:()=>o.Ah,updateLinkWithAccountId:()=>l.o,updateStore:()=>o.ow,upgradePlanLink:()=>L.Ps,upgradeWithCouponPlanLinkV1:()=>L.sD,upsellDataLink:()=>L.Mn,useStorageLog:()=>o.o7,xrayInvocationLink:()=>L.$w});var r=n(56639),o=n(89697),a=n(53738),i=n(91255),l=n(9393),s=(n(73212),n(47508)),u=n(98150),c=n(10256),d=n(88805);n.o(d,"BSR_ERROR_MESSAGE_MAP")&&n.d(t,{BSR_ERROR_MESSAGE_MAP:function(){return d.BSR_ERROR_MESSAGE_MAP}}),n.o(d,"FadeAnimation")&&n.d(t,{FadeAnimation:function(){return d.FadeAnimation}}),n.o(d,"MenuAsinGrabberButton")&&n.d(t,{MenuAsinGrabberButton:function(){return d.MenuAsinGrabberButton}}),n.o(d,"MenuCalculatorButton")&&n.d(t,{MenuCalculatorButton:function(){return d.MenuCalculatorButton}}),n.o(d,"MenuCalculatorWalmartButton")&&n.d(t,{MenuCalculatorWalmartButton:function(){return d.MenuCalculatorWalmartButton}}),n.o(d,"MenuFeedbackButton")&&n.d(t,{MenuFeedbackButton:function(){return d.MenuFeedbackButton}}),n.o(d,"MenuIconWrapper")&&n.d(t,{MenuIconWrapper:function(){return d.MenuIconWrapper}}),n.o(d,"MenuInventoryButton")&&n.d(t,{MenuInventoryButton:function(){return d.MenuInventoryButton}}),n.o(d,"MenuTiktokReviewInsightsButton")&&n.d(t,{MenuTiktokReviewInsightsButton:function(){return d.MenuTiktokReviewInsightsButton}}),n.o(d,"MenuXrayButton")&&n.d(t,{MenuXrayButton:function(){return d.MenuXrayButton}}),n.o(d,"MenuXrayKeywordsButton")&&n.d(t,{MenuXrayKeywordsButton:function(){return d.MenuXrayKeywordsButton}}),n.o(d,"MenuXrayWalmartButton")&&n.d(t,{MenuXrayWalmartButton:function(){return d.MenuXrayWalmartButton}}),n.o(d,"Modal")&&n.d(t,{Modal:function(){return d.Modal}}),n.o(d,"ModalWindowRoot")&&n.d(t,{ModalWindowRoot:function(){return d.ModalWindowRoot}}),n.o(d,"RootElement")&&n.d(t,{RootElement:function(){return d.RootElement}}),n.o(d,"ToastElement")&&n.d(t,{ToastElement:function(){return d.ToastElement}}),n.o(d,"TokenConnection")&&n.d(t,{TokenConnection:function(){return d.TokenConnection}}),n.o(d,"abaDataLink")&&n.d(t,{abaDataLink:function(){return d.abaDataLink}}),n.o(d,"addModalState")&&n.d(t,{addModalState:function(){return d.addModalState}}),n.o(d,"amazonLink")&&n.d(t,{amazonLink:function(){return d.amazonLink}}),n.o(d,"amazonUserSettingLink")&&n.d(t,{amazonUserSettingLink:function(){return d.amazonUserSettingLink}}),n.o(d,"bsrGraphDataLink")&&n.d(t,{bsrGraphDataLink:function(){return d.bsrGraphDataLink}}),n.o(d,"bsrGraphsSelector")&&n.d(t,{bsrGraphsSelector:function(){return d.bsrGraphsSelector}}),n.o(d,"checkIfBelowPlatinum")&&n.d(t,{checkIfBelowPlatinum:function(){return d.checkIfBelowPlatinum}}),n.o(d,"chromeStorageLocalPersister")&&n.d(t,{chromeStorageLocalPersister:function(){return d.chromeStorageLocalPersister}}),n.o(d,"controlCenterLink")&&n.d(t,{controlCenterLink:function(){return d.controlCenterLink}}),n.o(d,"dayMilliseconds")&&n.d(t,{dayMilliseconds:function(){return d.dayMilliseconds}}),n.o(d,"feedbackFormLink")&&n.d(t,{feedbackFormLink:function(){return d.feedbackFormLink}}),n.o(d,"feedbackSurvey")&&n.d(t,{feedbackSurvey:function(){return d.feedbackSurvey}}),n.o(d,"findCreateElement")&&n.d(t,{findCreateElement:function(){return d.findCreateElement}}),n.o(d,"formatPrice")&&n.d(t,{formatPrice:function(){return d.formatPrice}}),n.o(d,"get30DayRevenueByParent")&&n.d(t,{get30DayRevenueByParent:function(){return d.get30DayRevenueByParent}}),n.o(d,"get30DaySalesByParent")&&n.d(t,{get30DaySalesByParent:function(){return d.get30DaySalesByParent}}),n.o(d,"getAverageCalculations")&&n.d(t,{getAverageCalculations:function(){return d.getAverageCalculations}}),n.o(d,"getBSRDataCacheKey")&&n.d(t,{getBSRDataCacheKey:function(){return d.getBSRDataCacheKey}}),n.o(d,"getCheckLoginLink")&&n.d(t,{getCheckLoginLink:function(){return d.getCheckLoginLink}}),n.o(d,"getCommissionRate")&&n.d(t,{getCommissionRate:function(){return d.getCommissionRate}}),n.o(d,"getCountryLink")&&n.d(t,{getCountryLink:function(){return d.getCountryLink}}),n.o(d,"getCoupon")&&n.d(t,{getCoupon:function(){return d.getCoupon}}),n.o(d,"getCurrentMarketplace")&&n.d(t,{getCurrentMarketplace:function(){return d.getCurrentMarketplace}}),n.o(d,"getCurrentMarketplaceTLDomain")&&n.d(t,{getCurrentMarketplaceTLDomain:function(){return d.getCurrentMarketplaceTLDomain}}),n.o(d,"getDomain")&&n.d(t,{getDomain:function(){return d.getDomain}}),n.o(d,"getDomainFromUrl")&&n.d(t,{getDomainFromUrl:function(){return d.getDomainFromUrl}}),n.o(d,"getInfluencerVideoCount")&&n.d(t,{getInfluencerVideoCount:function(){return d.getInfluencerVideoCount}}),n.o(d,"getInterfaceLanguage")&&n.d(t,{getInterfaceLanguage:function(){return d.getInterfaceLanguage}}),n.o(d,"getInvocation")&&n.d(t,{getInvocation:function(){return d.getInvocation}}),n.o(d,"getLanguages")&&n.d(t,{getLanguages:function(){return d.getLanguages}}),n.o(d,"getMarketDataFromDomain")&&n.d(t,{getMarketDataFromDomain:function(){return d.getMarketDataFromDomain}}),n.o(d,"getMarketplaceCurrency")&&n.d(t,{getMarketplaceCurrency:function(){return d.getMarketplaceCurrency}}),n.o(d,"getModalState")&&n.d(t,{getModalState:function(){return d.getModalState}}),n.o(d,"getMyProductIdeaList")&&n.d(t,{getMyProductIdeaList:function(){return d.getMyProductIdeaList}}),n.o(d,"getProducts")&&n.d(t,{getProducts:function(){return d.getProducts}}),n.o(d,"getSalesGraph")&&n.d(t,{getSalesGraph:function(){return d.getSalesGraph}}),n.o(d,"getSellersCountries")&&n.d(t,{getSellersCountries:function(){return d.getSellersCountries}}),n.o(d,"getTiktokReviewChargeUseLink")&&n.d(t,{getTiktokReviewChargeUseLink:function(){return d.getTiktokReviewChargeUseLink}}),n.o(d,"getTranslation")&&n.d(t,{getTranslation:function(){return d.getTranslation}}),n.o(d,"getUpgradeWithCouponPlanLinkV1")&&n.d(t,{getUpgradeWithCouponPlanLinkV1:function(){return d.getUpgradeWithCouponPlanLinkV1}}),n.o(d,"getUpgradeWithCouponPlanLinkV2")&&n.d(t,{getUpgradeWithCouponPlanLinkV2:function(){return d.getUpgradeWithCouponPlanLinkV2}}),n.o(d,"graphsStore")&&n.d(t,{graphsStore:function(){return d.graphsStore}}),n.o(d,"heliumExtensionInStore")&&n.d(t,{heliumExtensionInStore:function(){return d.heliumExtensionInStore}}),n.o(d,"isPositiveValue")&&n.d(t,{isPositiveValue:function(){return d.isPositiveValue}}),n.o(d,"isReadyStatus")&&n.d(t,{isReadyStatus:function(){return d.isReadyStatus}}),n.o(d,"isWalmartProductPageNew")&&n.d(t,{isWalmartProductPageNew:function(){return d.isWalmartProductPageNew}}),n.o(d,"isWalmartSearchPage")&&n.d(t,{isWalmartSearchPage:function(){return d.isWalmartSearchPage}}),n.o(d,"learnHowToUseLink")&&n.d(t,{learnHowToUseLink:function(){return d.learnHowToUseLink}}),n.o(d,"mainHeliumLink")&&n.d(t,{mainHeliumLink:function(){return d.mainHeliumLink}}),n.o(d,"membersLink")&&n.d(t,{membersLink:function(){return d.membersLink}}),n.o(d,"parserConfigLink")&&n.d(t,{parserConfigLink:function(){return d.parserConfigLink}}),n.o(d,"priceFromApi")&&n.d(t,{priceFromApi:function(){return d.priceFromApi}}),n.o(d,"queryClient")&&n.d(t,{queryClient:function(){return d.queryClient}}),n.o(d,"realTokenLink")&&n.d(t,{realTokenLink:function(){return d.realTokenLink}}),n.o(d,"reduceAccountInfo")&&n.d(t,{reduceAccountInfo:function(){return d.reduceAccountInfo}}),n.o(d,"reduceCheckLogin")&&n.d(t,{reduceCheckLogin:function(){return d.reduceCheckLogin}}),n.o(d,"renderElement")&&n.d(t,{renderElement:function(){return d.renderElement}}),n.o(d,"researchLink")&&n.d(t,{researchLink:function(){return d.researchLink}}),n.o(d,"reviewGraphDataLink")&&n.d(t,{reviewGraphDataLink:function(){return d.reviewGraphDataLink}}),n.o(d,"salesGraphDataLink")&&n.d(t,{salesGraphDataLink:function(){return d.salesGraphDataLink}}),n.o(d,"salesLink")&&n.d(t,{salesLink:function(){return d.salesLink}}),n.o(d,"searchGoogleGraphDataLink")&&n.d(t,{searchGoogleGraphDataLink:function(){return d.searchGoogleGraphDataLink}}),n.o(d,"signupLink")&&n.d(t,{signupLink:function(){return d.signupLink}}),n.o(d,"toggleExtension")&&n.d(t,{toggleExtension:function(){return d.toggleExtension}}),n.o(d,"tokenConnectionLink")&&n.d(t,{tokenConnectionLink:function(){return d.tokenConnectionLink}}),n.o(d,"transReactQueryStatus")&&n.d(t,{transReactQueryStatus:function(){return d.transReactQueryStatus}}),n.o(d,"transformIntoCNLink")&&n.d(t,{transformIntoCNLink:function(){return d.transformIntoCNLink}}),n.o(d,"upgradePlanLink")&&n.d(t,{upgradePlanLink:function(){return d.upgradePlanLink}}),n.o(d,"upgradeWithCouponPlanLinkV1")&&n.d(t,{upgradeWithCouponPlanLinkV1:function(){return d.upgradeWithCouponPlanLinkV1}}),n.o(d,"upsellDataLink")&&n.d(t,{upsellDataLink:function(){return d.upsellDataLink}}),n.o(d,"useEntitlements")&&n.d(t,{useEntitlements:function(){return d.useEntitlements}}),n.o(d,"useGetCacheStore")&&n.d(t,{useGetCacheStore:function(){return d.useGetCacheStore}}),n.o(d,"useGetStore")&&n.d(t,{useGetStore:function(){return d.useGetStore}}),n.o(d,"useModalState")&&n.d(t,{useModalState:function(){return d.useModalState}}),n.o(d,"useShowSpinner")&&n.d(t,{useShowSpinner:function(){return d.useShowSpinner}}),n.o(d,"useTranslation")&&n.d(t,{useTranslation:function(){return d.useTranslation}}),n.o(d,"waitForProduct")&&n.d(t,{waitForProduct:function(){return d.waitForProduct}}),n.o(d,"waitForSalesData")&&n.d(t,{waitForSalesData:function(){return d.waitForSalesData}}),n.o(d,"xrayInvocationLink")&&n.d(t,{xrayInvocationLink:function(){return d.xrayInvocationLink}});var p=n(99650),g=n(14833),h=n(79802);n.o(h,"BSR_ERROR_MESSAGE_MAP")&&n.d(t,{BSR_ERROR_MESSAGE_MAP:function(){return h.BSR_ERROR_MESSAGE_MAP}}),n.o(h,"FadeAnimation")&&n.d(t,{FadeAnimation:function(){return h.FadeAnimation}}),n.o(h,"MenuAsinGrabberButton")&&n.d(t,{MenuAsinGrabberButton:function(){return h.MenuAsinGrabberButton}}),n.o(h,"MenuCalculatorButton")&&n.d(t,{MenuCalculatorButton:function(){return h.MenuCalculatorButton}}),n.o(h,"MenuCalculatorWalmartButton")&&n.d(t,{MenuCalculatorWalmartButton:function(){return h.MenuCalculatorWalmartButton}}),n.o(h,"MenuFeedbackButton")&&n.d(t,{MenuFeedbackButton:function(){return h.MenuFeedbackButton}}),n.o(h,"MenuIconWrapper")&&n.d(t,{MenuIconWrapper:function(){return h.MenuIconWrapper}}),n.o(h,"MenuInventoryButton")&&n.d(t,{MenuInventoryButton:function(){return h.MenuInventoryButton}}),n.o(h,"MenuTiktokReviewInsightsButton")&&n.d(t,{MenuTiktokReviewInsightsButton:function(){return h.MenuTiktokReviewInsightsButton}}),n.o(h,"MenuXrayButton")&&n.d(t,{MenuXrayButton:function(){return h.MenuXrayButton}}),n.o(h,"MenuXrayKeywordsButton")&&n.d(t,{MenuXrayKeywordsButton:function(){return h.MenuXrayKeywordsButton}}),n.o(h,"MenuXrayWalmartButton")&&n.d(t,{MenuXrayWalmartButton:function(){return h.MenuXrayWalmartButton}}),n.o(h,"Modal")&&n.d(t,{Modal:function(){return h.Modal}}),n.o(h,"ModalWindowRoot")&&n.d(t,{ModalWindowRoot:function(){return h.ModalWindowRoot}}),n.o(h,"RootElement")&&n.d(t,{RootElement:function(){return h.RootElement}}),n.o(h,"ToastElement")&&n.d(t,{ToastElement:function(){return h.ToastElement}}),n.o(h,"TokenConnection")&&n.d(t,{TokenConnection:function(){return h.TokenConnection}}),n.o(h,"abaDataLink")&&n.d(t,{abaDataLink:function(){return h.abaDataLink}}),n.o(h,"addModalState")&&n.d(t,{addModalState:function(){return h.addModalState}}),n.o(h,"amazonLink")&&n.d(t,{amazonLink:function(){return h.amazonLink}}),n.o(h,"amazonUserSettingLink")&&n.d(t,{amazonUserSettingLink:function(){return h.amazonUserSettingLink}}),n.o(h,"bsrGraphDataLink")&&n.d(t,{bsrGraphDataLink:function(){return h.bsrGraphDataLink}}),n.o(h,"bsrGraphsSelector")&&n.d(t,{bsrGraphsSelector:function(){return h.bsrGraphsSelector}}),n.o(h,"checkIfBelowPlatinum")&&n.d(t,{checkIfBelowPlatinum:function(){return h.checkIfBelowPlatinum}}),n.o(h,"chromeStorageLocalPersister")&&n.d(t,{chromeStorageLocalPersister:function(){return h.chromeStorageLocalPersister}}),n.o(h,"controlCenterLink")&&n.d(t,{controlCenterLink:function(){return h.controlCenterLink}}),n.o(h,"dayMilliseconds")&&n.d(t,{dayMilliseconds:function(){return h.dayMilliseconds}}),n.o(h,"feedbackFormLink")&&n.d(t,{feedbackFormLink:function(){return h.feedbackFormLink}}),n.o(h,"feedbackSurvey")&&n.d(t,{feedbackSurvey:function(){return h.feedbackSurvey}}),n.o(h,"findCreateElement")&&n.d(t,{findCreateElement:function(){return h.findCreateElement}}),n.o(h,"formatPrice")&&n.d(t,{formatPrice:function(){return h.formatPrice}}),n.o(h,"get30DayRevenueByParent")&&n.d(t,{get30DayRevenueByParent:function(){return h.get30DayRevenueByParent}}),n.o(h,"get30DaySalesByParent")&&n.d(t,{get30DaySalesByParent:function(){return h.get30DaySalesByParent}}),n.o(h,"getAverageCalculations")&&n.d(t,{getAverageCalculations:function(){return h.getAverageCalculations}}),n.o(h,"getCheckLoginLink")&&n.d(t,{getCheckLoginLink:function(){return h.getCheckLoginLink}}),n.o(h,"getCommissionRate")&&n.d(t,{getCommissionRate:function(){return h.getCommissionRate}}),n.o(h,"getCountryLink")&&n.d(t,{getCountryLink:function(){return h.getCountryLink}}),n.o(h,"getCoupon")&&n.d(t,{getCoupon:function(){return h.getCoupon}}),n.o(h,"getCurrentMarketplace")&&n.d(t,{getCurrentMarketplace:function(){return h.getCurrentMarketplace}}),n.o(h,"getCurrentMarketplaceTLDomain")&&n.d(t,{getCurrentMarketplaceTLDomain:function(){return h.getCurrentMarketplaceTLDomain}}),n.o(h,"getDomain")&&n.d(t,{getDomain:function(){return h.getDomain}}),n.o(h,"getDomainFromUrl")&&n.d(t,{getDomainFromUrl:function(){return h.getDomainFromUrl}}),n.o(h,"getInfluencerVideoCount")&&n.d(t,{getInfluencerVideoCount:function(){return h.getInfluencerVideoCount}}),n.o(h,"getInterfaceLanguage")&&n.d(t,{getInterfaceLanguage:function(){return h.getInterfaceLanguage}}),n.o(h,"getInvocation")&&n.d(t,{getInvocation:function(){return h.getInvocation}}),n.o(h,"getLanguages")&&n.d(t,{getLanguages:function(){return h.getLanguages}}),n.o(h,"getMarketDataFromDomain")&&n.d(t,{getMarketDataFromDomain:function(){return h.getMarketDataFromDomain}}),n.o(h,"getMarketplaceCurrency")&&n.d(t,{getMarketplaceCurrency:function(){return h.getMarketplaceCurrency}}),n.o(h,"getModalState")&&n.d(t,{getModalState:function(){return h.getModalState}}),n.o(h,"getMyProductIdeaList")&&n.d(t,{getMyProductIdeaList:function(){return h.getMyProductIdeaList}}),n.o(h,"getProducts")&&n.d(t,{getProducts:function(){return h.getProducts}}),n.o(h,"getSalesGraph")&&n.d(t,{getSalesGraph:function(){return h.getSalesGraph}}),n.o(h,"getSellersCountries")&&n.d(t,{getSellersCountries:function(){return h.getSellersCountries}}),n.o(h,"getTiktokReviewChargeUseLink")&&n.d(t,{getTiktokReviewChargeUseLink:function(){return h.getTiktokReviewChargeUseLink}}),n.o(h,"getTranslation")&&n.d(t,{getTranslation:function(){return h.getTranslation}}),n.o(h,"getUpgradeWithCouponPlanLinkV1")&&n.d(t,{getUpgradeWithCouponPlanLinkV1:function(){return h.getUpgradeWithCouponPlanLinkV1}}),n.o(h,"getUpgradeWithCouponPlanLinkV2")&&n.d(t,{getUpgradeWithCouponPlanLinkV2:function(){return h.getUpgradeWithCouponPlanLinkV2}}),n.o(h,"graphsStore")&&n.d(t,{graphsStore:function(){return h.graphsStore}}),n.o(h,"heliumExtensionInStore")&&n.d(t,{heliumExtensionInStore:function(){return h.heliumExtensionInStore}}),n.o(h,"isPositiveValue")&&n.d(t,{isPositiveValue:function(){return h.isPositiveValue}}),n.o(h,"isReadyStatus")&&n.d(t,{isReadyStatus:function(){return h.isReadyStatus}}),n.o(h,"isWalmartProductPageNew")&&n.d(t,{isWalmartProductPageNew:function(){return h.isWalmartProductPageNew}}),n.o(h,"isWalmartSearchPage")&&n.d(t,{isWalmartSearchPage:function(){return h.isWalmartSearchPage}}),n.o(h,"learnHowToUseLink")&&n.d(t,{learnHowToUseLink:function(){return h.learnHowToUseLink}}),n.o(h,"mainHeliumLink")&&n.d(t,{mainHeliumLink:function(){return h.mainHeliumLink}}),n.o(h,"membersLink")&&n.d(t,{membersLink:function(){return h.membersLink}}),n.o(h,"parserConfigLink")&&n.d(t,{parserConfigLink:function(){return h.parserConfigLink}}),n.o(h,"priceFromApi")&&n.d(t,{priceFromApi:function(){return h.priceFromApi}}),n.o(h,"queryClient")&&n.d(t,{queryClient:function(){return h.queryClient}}),n.o(h,"realTokenLink")&&n.d(t,{realTokenLink:function(){return h.realTokenLink}}),n.o(h,"reduceCheckLogin")&&n.d(t,{reduceCheckLogin:function(){return h.reduceCheckLogin}}),n.o(h,"renderElement")&&n.d(t,{renderElement:function(){return h.renderElement}}),n.o(h,"researchLink")&&n.d(t,{researchLink:function(){return h.researchLink}}),n.o(h,"reviewGraphDataLink")&&n.d(t,{reviewGraphDataLink:function(){return h.reviewGraphDataLink}}),n.o(h,"salesGraphDataLink")&&n.d(t,{salesGraphDataLink:function(){return h.salesGraphDataLink}}),n.o(h,"salesLink")&&n.d(t,{salesLink:function(){return h.salesLink}}),n.o(h,"searchGoogleGraphDataLink")&&n.d(t,{searchGoogleGraphDataLink:function(){return h.searchGoogleGraphDataLink}}),n.o(h,"signupLink")&&n.d(t,{signupLink:function(){return h.signupLink}}),n.o(h,"toggleExtension")&&n.d(t,{toggleExtension:function(){return h.toggleExtension}}),n.o(h,"tokenConnectionLink")&&n.d(t,{tokenConnectionLink:function(){return h.tokenConnectionLink}}),n.o(h,"transReactQueryStatus")&&n.d(t,{transReactQueryStatus:function(){return h.transReactQueryStatus}}),n.o(h,"transformIntoCNLink")&&n.d(t,{transformIntoCNLink:function(){return h.transformIntoCNLink}}),n.o(h,"upgradePlanLink")&&n.d(t,{upgradePlanLink:function(){return h.upgradePlanLink}}),n.o(h,"upgradeWithCouponPlanLinkV1")&&n.d(t,{upgradeWithCouponPlanLinkV1:function(){return h.upgradeWithCouponPlanLinkV1}}),n.o(h,"upsellDataLink")&&n.d(t,{upsellDataLink:function(){return h.upsellDataLink}}),n.o(h,"useEntitlements")&&n.d(t,{useEntitlements:function(){return h.useEntitlements}}),n.o(h,"useGetCacheStore")&&n.d(t,{useGetCacheStore:function(){return h.useGetCacheStore}}),n.o(h,"useGetStore")&&n.d(t,{useGetStore:function(){return h.useGetStore}}),n.o(h,"useModalState")&&n.d(t,{useModalState:function(){return h.useModalState}}),n.o(h,"useShowSpinner")&&n.d(t,{useShowSpinner:function(){return h.useShowSpinner}}),n.o(h,"useTranslation")&&n.d(t,{useTranslation:function(){return h.useTranslation}}),n.o(h,"waitForProduct")&&n.d(t,{waitForProduct:function(){return h.waitForProduct}}),n.o(h,"waitForSalesData")&&n.d(t,{waitForSalesData:function(){return h.waitForSalesData}}),n.o(h,"xrayInvocationLink")&&n.d(t,{xrayInvocationLink:function(){return h.xrayInvocationLink}});var m=n(47801);n.o(m,"BSR_ERROR_MESSAGE_MAP")&&n.d(t,{BSR_ERROR_MESSAGE_MAP:function(){return m.BSR_ERROR_MESSAGE_MAP}}),n.o(m,"FadeAnimation")&&n.d(t,{FadeAnimation:function(){return m.FadeAnimation}}),n.o(m,"MenuAsinGrabberButton")&&n.d(t,{MenuAsinGrabberButton:function(){return m.MenuAsinGrabberButton}}),n.o(m,"MenuCalculatorButton")&&n.d(t,{MenuCalculatorButton:function(){return m.MenuCalculatorButton}}),n.o(m,"MenuCalculatorWalmartButton")&&n.d(t,{MenuCalculatorWalmartButton:function(){return m.MenuCalculatorWalmartButton}}),n.o(m,"MenuFeedbackButton")&&n.d(t,{MenuFeedbackButton:function(){return m.MenuFeedbackButton}}),n.o(m,"MenuIconWrapper")&&n.d(t,{MenuIconWrapper:function(){return m.MenuIconWrapper}}),n.o(m,"MenuInventoryButton")&&n.d(t,{MenuInventoryButton:function(){return m.MenuInventoryButton}}),n.o(m,"MenuTiktokReviewInsightsButton")&&n.d(t,{MenuTiktokReviewInsightsButton:function(){return m.MenuTiktokReviewInsightsButton}}),n.o(m,"MenuXrayButton")&&n.d(t,{MenuXrayButton:function(){return m.MenuXrayButton}}),n.o(m,"MenuXrayKeywordsButton")&&n.d(t,{MenuXrayKeywordsButton:function(){return m.MenuXrayKeywordsButton}}),n.o(m,"MenuXrayWalmartButton")&&n.d(t,{MenuXrayWalmartButton:function(){return m.MenuXrayWalmartButton}}),n.o(m,"Modal")&&n.d(t,{Modal:function(){return m.Modal}}),n.o(m,"ModalWindowRoot")&&n.d(t,{ModalWindowRoot:function(){return m.ModalWindowRoot}}),n.o(m,"RootElement")&&n.d(t,{RootElement:function(){return m.RootElement}}),n.o(m,"ToastElement")&&n.d(t,{ToastElement:function(){return m.ToastElement}}),n.o(m,"TokenConnection")&&n.d(t,{TokenConnection:function(){return m.TokenConnection}}),n.o(m,"abaDataLink")&&n.d(t,{abaDataLink:function(){return m.abaDataLink}}),n.o(m,"addModalState")&&n.d(t,{addModalState:function(){return m.addModalState}}),n.o(m,"amazonLink")&&n.d(t,{amazonLink:function(){return m.amazonLink}}),n.o(m,"amazonUserSettingLink")&&n.d(t,{amazonUserSettingLink:function(){return m.amazonUserSettingLink}}),n.o(m,"bsrGraphDataLink")&&n.d(t,{bsrGraphDataLink:function(){return m.bsrGraphDataLink}}),n.o(m,"bsrGraphsSelector")&&n.d(t,{bsrGraphsSelector:function(){return m.bsrGraphsSelector}}),n.o(m,"checkIfBelowPlatinum")&&n.d(t,{checkIfBelowPlatinum:function(){return m.checkIfBelowPlatinum}}),n.o(m,"chromeStorageLocalPersister")&&n.d(t,{chromeStorageLocalPersister:function(){return m.chromeStorageLocalPersister}}),n.o(m,"controlCenterLink")&&n.d(t,{controlCenterLink:function(){return m.controlCenterLink}}),n.o(m,"dayMilliseconds")&&n.d(t,{dayMilliseconds:function(){return m.dayMilliseconds}}),n.o(m,"feedbackFormLink")&&n.d(t,{feedbackFormLink:function(){return m.feedbackFormLink}}),n.o(m,"feedbackSurvey")&&n.d(t,{feedbackSurvey:function(){return m.feedbackSurvey}}),n.o(m,"findCreateElement")&&n.d(t,{findCreateElement:function(){return m.findCreateElement}}),n.o(m,"formatPrice")&&n.d(t,{formatPrice:function(){return m.formatPrice}}),n.o(m,"get30DayRevenueByParent")&&n.d(t,{get30DayRevenueByParent:function(){return m.get30DayRevenueByParent}}),n.o(m,"get30DaySalesByParent")&&n.d(t,{get30DaySalesByParent:function(){return m.get30DaySalesByParent}}),n.o(m,"getAverageCalculations")&&n.d(t,{getAverageCalculations:function(){return m.getAverageCalculations}}),n.o(m,"getCheckLoginLink")&&n.d(t,{getCheckLoginLink:function(){return m.getCheckLoginLink}}),n.o(m,"getCommissionRate")&&n.d(t,{getCommissionRate:function(){return m.getCommissionRate}}),n.o(m,"getCountryLink")&&n.d(t,{getCountryLink:function(){return m.getCountryLink}}),n.o(m,"getCoupon")&&n.d(t,{getCoupon:function(){return m.getCoupon}}),n.o(m,"getCurrentMarketplace")&&n.d(t,{getCurrentMarketplace:function(){return m.getCurrentMarketplace}}),n.o(m,"getCurrentMarketplaceTLDomain")&&n.d(t,{getCurrentMarketplaceTLDomain:function(){return m.getCurrentMarketplaceTLDomain}}),n.o(m,"getDomain")&&n.d(t,{getDomain:function(){return m.getDomain}}),n.o(m,"getDomainFromUrl")&&n.d(t,{getDomainFromUrl:function(){return m.getDomainFromUrl}}),n.o(m,"getInfluencerVideoCount")&&n.d(t,{getInfluencerVideoCount:function(){return m.getInfluencerVideoCount}}),n.o(m,"getInterfaceLanguage")&&n.d(t,{getInterfaceLanguage:function(){return m.getInterfaceLanguage}}),n.o(m,"getInvocation")&&n.d(t,{getInvocation:function(){return m.getInvocation}}),n.o(m,"getLanguages")&&n.d(t,{getLanguages:function(){return m.getLanguages}}),n.o(m,"getMarketDataFromDomain")&&n.d(t,{getMarketDataFromDomain:function(){return m.getMarketDataFromDomain}}),n.o(m,"getMarketplaceCurrency")&&n.d(t,{getMarketplaceCurrency:function(){return m.getMarketplaceCurrency}}),n.o(m,"getModalState")&&n.d(t,{getModalState:function(){return m.getModalState}}),n.o(m,"getMyProductIdeaList")&&n.d(t,{getMyProductIdeaList:function(){return m.getMyProductIdeaList}}),n.o(m,"getProducts")&&n.d(t,{getProducts:function(){return m.getProducts}}),n.o(m,"getSalesGraph")&&n.d(t,{getSalesGraph:function(){return m.getSalesGraph}}),n.o(m,"getSellersCountries")&&n.d(t,{getSellersCountries:function(){return m.getSellersCountries}}),n.o(m,"getTiktokReviewChargeUseLink")&&n.d(t,{getTiktokReviewChargeUseLink:function(){return m.getTiktokReviewChargeUseLink}}),n.o(m,"getTranslation")&&n.d(t,{getTranslation:function(){return m.getTranslation}}),n.o(m,"getUpgradeWithCouponPlanLinkV1")&&n.d(t,{getUpgradeWithCouponPlanLinkV1:function(){return m.getUpgradeWithCouponPlanLinkV1}}),n.o(m,"getUpgradeWithCouponPlanLinkV2")&&n.d(t,{getUpgradeWithCouponPlanLinkV2:function(){return m.getUpgradeWithCouponPlanLinkV2}}),n.o(m,"graphsStore")&&n.d(t,{graphsStore:function(){return m.graphsStore}}),n.o(m,"heliumExtensionInStore")&&n.d(t,{heliumExtensionInStore:function(){return m.heliumExtensionInStore}}),n.o(m,"isPositiveValue")&&n.d(t,{isPositiveValue:function(){return m.isPositiveValue}}),n.o(m,"isReadyStatus")&&n.d(t,{isReadyStatus:function(){return m.isReadyStatus}}),n.o(m,"isWalmartProductPageNew")&&n.d(t,{isWalmartProductPageNew:function(){return m.isWalmartProductPageNew}}),n.o(m,"isWalmartSearchPage")&&n.d(t,{isWalmartSearchPage:function(){return m.isWalmartSearchPage}}),n.o(m,"learnHowToUseLink")&&n.d(t,{learnHowToUseLink:function(){return m.learnHowToUseLink}}),n.o(m,"mainHeliumLink")&&n.d(t,{mainHeliumLink:function(){return m.mainHeliumLink}}),n.o(m,"membersLink")&&n.d(t,{membersLink:function(){return m.membersLink}}),n.o(m,"parserConfigLink")&&n.d(t,{parserConfigLink:function(){return m.parserConfigLink}}),n.o(m,"priceFromApi")&&n.d(t,{priceFromApi:function(){return m.priceFromApi}}),n.o(m,"queryClient")&&n.d(t,{queryClient:function(){return m.queryClient}}),n.o(m,"realTokenLink")&&n.d(t,{realTokenLink:function(){return m.realTokenLink}}),n.o(m,"reduceCheckLogin")&&n.d(t,{reduceCheckLogin:function(){return m.reduceCheckLogin}}),n.o(m,"renderElement")&&n.d(t,{renderElement:function(){return m.renderElement}}),n.o(m,"researchLink")&&n.d(t,{researchLink:function(){return m.researchLink}}),n.o(m,"reviewGraphDataLink")&&n.d(t,{reviewGraphDataLink:function(){return m.reviewGraphDataLink}}),n.o(m,"salesGraphDataLink")&&n.d(t,{salesGraphDataLink:function(){return m.salesGraphDataLink}}),n.o(m,"salesLink")&&n.d(t,{salesLink:function(){return m.salesLink}}),n.o(m,"searchGoogleGraphDataLink")&&n.d(t,{searchGoogleGraphDataLink:function(){return m.searchGoogleGraphDataLink}}),n.o(m,"signupLink")&&n.d(t,{signupLink:function(){return m.signupLink}}),n.o(m,"toggleExtension")&&n.d(t,{toggleExtension:function(){return m.toggleExtension}}),n.o(m,"tokenConnectionLink")&&n.d(t,{tokenConnectionLink:function(){return m.tokenConnectionLink}}),n.o(m,"transReactQueryStatus")&&n.d(t,{transReactQueryStatus:function(){return m.transReactQueryStatus}}),n.o(m,"transformIntoCNLink")&&n.d(t,{transformIntoCNLink:function(){return m.transformIntoCNLink}}),n.o(m,"upgradePlanLink")&&n.d(t,{upgradePlanLink:function(){return m.upgradePlanLink}}),n.o(m,"upgradeWithCouponPlanLinkV1")&&n.d(t,{upgradeWithCouponPlanLinkV1:function(){return m.upgradeWithCouponPlanLinkV1}}),n.o(m,"upsellDataLink")&&n.d(t,{upsellDataLink:function(){return m.upsellDataLink}}),n.o(m,"useEntitlements")&&n.d(t,{useEntitlements:function(){return m.useEntitlements}}),n.o(m,"useGetCacheStore")&&n.d(t,{useGetCacheStore:function(){return m.useGetCacheStore}}),n.o(m,"useGetStore")&&n.d(t,{useGetStore:function(){return m.useGetStore}}),n.o(m,"useModalState")&&n.d(t,{useModalState:function(){return m.useModalState}}),n.o(m,"useShowSpinner")&&n.d(t,{useShowSpinner:function(){return m.useShowSpinner}}),n.o(m,"useTranslation")&&n.d(t,{useTranslation:function(){return m.useTranslation}}),n.o(m,"waitForProduct")&&n.d(t,{waitForProduct:function(){return m.waitForProduct}}),n.o(m,"waitForSalesData")&&n.d(t,{waitForSalesData:function(){return m.waitForSalesData}}),n.o(m,"xrayInvocationLink")&&n.d(t,{xrayInvocationLink:function(){return m.xrayInvocationLink}});var f=n(73743),v=n(79939);n.o(v,"BSR_ERROR_MESSAGE_MAP")&&n.d(t,{BSR_ERROR_MESSAGE_MAP:function(){return v.BSR_ERROR_MESSAGE_MAP}}),n.o(v,"FadeAnimation")&&n.d(t,{FadeAnimation:function(){return v.FadeAnimation}}),n.o(v,"MenuAsinGrabberButton")&&n.d(t,{MenuAsinGrabberButton:function(){return v.MenuAsinGrabberButton}}),n.o(v,"MenuCalculatorButton")&&n.d(t,{MenuCalculatorButton:function(){return v.MenuCalculatorButton}}),n.o(v,"MenuCalculatorWalmartButton")&&n.d(t,{MenuCalculatorWalmartButton:function(){return v.MenuCalculatorWalmartButton}}),n.o(v,"MenuFeedbackButton")&&n.d(t,{MenuFeedbackButton:function(){return v.MenuFeedbackButton}}),n.o(v,"MenuIconWrapper")&&n.d(t,{MenuIconWrapper:function(){return v.MenuIconWrapper}}),n.o(v,"MenuInventoryButton")&&n.d(t,{MenuInventoryButton:function(){return v.MenuInventoryButton}}),n.o(v,"MenuTiktokReviewInsightsButton")&&n.d(t,{MenuTiktokReviewInsightsButton:function(){return v.MenuTiktokReviewInsightsButton}}),n.o(v,"MenuXrayButton")&&n.d(t,{MenuXrayButton:function(){return v.MenuXrayButton}}),n.o(v,"MenuXrayKeywordsButton")&&n.d(t,{MenuXrayKeywordsButton:function(){return v.MenuXrayKeywordsButton}}),n.o(v,"MenuXrayWalmartButton")&&n.d(t,{MenuXrayWalmartButton:function(){return v.MenuXrayWalmartButton}}),n.o(v,"Modal")&&n.d(t,{Modal:function(){return v.Modal}}),n.o(v,"ModalWindowRoot")&&n.d(t,{ModalWindowRoot:function(){return v.ModalWindowRoot}}),n.o(v,"RootElement")&&n.d(t,{RootElement:function(){return v.RootElement}}),n.o(v,"ToastElement")&&n.d(t,{ToastElement:function(){return v.ToastElement}}),n.o(v,"TokenConnection")&&n.d(t,{TokenConnection:function(){return v.TokenConnection}}),n.o(v,"abaDataLink")&&n.d(t,{abaDataLink:function(){return v.abaDataLink}}),n.o(v,"addModalState")&&n.d(t,{addModalState:function(){return v.addModalState}}),n.o(v,"amazonLink")&&n.d(t,{amazonLink:function(){return v.amazonLink}}),n.o(v,"amazonUserSettingLink")&&n.d(t,{amazonUserSettingLink:function(){return v.amazonUserSettingLink}}),n.o(v,"bsrGraphDataLink")&&n.d(t,{bsrGraphDataLink:function(){return v.bsrGraphDataLink}}),n.o(v,"bsrGraphsSelector")&&n.d(t,{bsrGraphsSelector:function(){return v.bsrGraphsSelector}}),n.o(v,"checkIfBelowPlatinum")&&n.d(t,{checkIfBelowPlatinum:function(){return v.checkIfBelowPlatinum}}),n.o(v,"chromeStorageLocalPersister")&&n.d(t,{chromeStorageLocalPersister:function(){return v.chromeStorageLocalPersister}}),n.o(v,"controlCenterLink")&&n.d(t,{controlCenterLink:function(){return v.controlCenterLink}}),n.o(v,"dayMilliseconds")&&n.d(t,{dayMilliseconds:function(){return v.dayMilliseconds}}),n.o(v,"feedbackFormLink")&&n.d(t,{feedbackFormLink:function(){return v.feedbackFormLink}}),n.o(v,"feedbackSurvey")&&n.d(t,{feedbackSurvey:function(){return v.feedbackSurvey}}),n.o(v,"findCreateElement")&&n.d(t,{findCreateElement:function(){return v.findCreateElement}}),n.o(v,"formatPrice")&&n.d(t,{formatPrice:function(){return v.formatPrice}}),n.o(v,"get30DayRevenueByParent")&&n.d(t,{get30DayRevenueByParent:function(){return v.get30DayRevenueByParent}}),n.o(v,"get30DaySalesByParent")&&n.d(t,{get30DaySalesByParent:function(){return v.get30DaySalesByParent}}),n.o(v,"getAverageCalculations")&&n.d(t,{getAverageCalculations:function(){return v.getAverageCalculations}}),n.o(v,"getCheckLoginLink")&&n.d(t,{getCheckLoginLink:function(){return v.getCheckLoginLink}}),n.o(v,"getCommissionRate")&&n.d(t,{getCommissionRate:function(){return v.getCommissionRate}}),n.o(v,"getCountryLink")&&n.d(t,{getCountryLink:function(){return v.getCountryLink}}),n.o(v,"getCoupon")&&n.d(t,{getCoupon:function(){return v.getCoupon}}),n.o(v,"getCurrentMarketplace")&&n.d(t,{getCurrentMarketplace:function(){return v.getCurrentMarketplace}}),n.o(v,"getCurrentMarketplaceTLDomain")&&n.d(t,{getCurrentMarketplaceTLDomain:function(){return v.getCurrentMarketplaceTLDomain}}),n.o(v,"getDomain")&&n.d(t,{getDomain:function(){return v.getDomain}}),n.o(v,"getDomainFromUrl")&&n.d(t,{getDomainFromUrl:function(){return v.getDomainFromUrl}}),n.o(v,"getInfluencerVideoCount")&&n.d(t,{getInfluencerVideoCount:function(){return v.getInfluencerVideoCount}}),n.o(v,"getInterfaceLanguage")&&n.d(t,{getInterfaceLanguage:function(){return v.getInterfaceLanguage}}),n.o(v,"getInvocation")&&n.d(t,{getInvocation:function(){return v.getInvocation}}),n.o(v,"getLanguages")&&n.d(t,{getLanguages:function(){return v.getLanguages}}),n.o(v,"getMarketDataFromDomain")&&n.d(t,{getMarketDataFromDomain:function(){return v.getMarketDataFromDomain}}),n.o(v,"getMarketplaceCurrency")&&n.d(t,{getMarketplaceCurrency:function(){return v.getMarketplaceCurrency}}),n.o(v,"getModalState")&&n.d(t,{getModalState:function(){return v.getModalState}}),n.o(v,"getMyProductIdeaList")&&n.d(t,{getMyProductIdeaList:function(){return v.getMyProductIdeaList}}),n.o(v,"getProducts")&&n.d(t,{getProducts:function(){return v.getProducts}}),n.o(v,"getSalesGraph")&&n.d(t,{getSalesGraph:function(){return v.getSalesGraph}}),n.o(v,"getSellersCountries")&&n.d(t,{getSellersCountries:function(){return v.getSellersCountries}}),n.o(v,"getTiktokReviewChargeUseLink")&&n.d(t,{getTiktokReviewChargeUseLink:function(){return v.getTiktokReviewChargeUseLink}}),n.o(v,"getTranslation")&&n.d(t,{getTranslation:function(){return v.getTranslation}}),n.o(v,"getUpgradeWithCouponPlanLinkV1")&&n.d(t,{getUpgradeWithCouponPlanLinkV1:function(){return v.getUpgradeWithCouponPlanLinkV1}}),n.o(v,"getUpgradeWithCouponPlanLinkV2")&&n.d(t,{getUpgradeWithCouponPlanLinkV2:function(){return v.getUpgradeWithCouponPlanLinkV2}}),n.o(v,"graphsStore")&&n.d(t,{graphsStore:function(){return v.graphsStore}}),n.o(v,"heliumExtensionInStore")&&n.d(t,{heliumExtensionInStore:function(){return v.heliumExtensionInStore}}),n.o(v,"isPositiveValue")&&n.d(t,{isPositiveValue:function(){return v.isPositiveValue}}),n.o(v,"isReadyStatus")&&n.d(t,{isReadyStatus:function(){return v.isReadyStatus}}),n.o(v,"isWalmartProductPageNew")&&n.d(t,{isWalmartProductPageNew:function(){return v.isWalmartProductPageNew}}),n.o(v,"isWalmartSearchPage")&&n.d(t,{isWalmartSearchPage:function(){return v.isWalmartSearchPage}}),n.o(v,"learnHowToUseLink")&&n.d(t,{learnHowToUseLink:function(){return v.learnHowToUseLink}}),n.o(v,"mainHeliumLink")&&n.d(t,{mainHeliumLink:function(){return v.mainHeliumLink}}),n.o(v,"membersLink")&&n.d(t,{membersLink:function(){return v.membersLink}}),n.o(v,"parserConfigLink")&&n.d(t,{parserConfigLink:function(){return v.parserConfigLink}}),n.o(v,"priceFromApi")&&n.d(t,{priceFromApi:function(){return v.priceFromApi}}),n.o(v,"queryClient")&&n.d(t,{queryClient:function(){return v.queryClient}}),n.o(v,"realTokenLink")&&n.d(t,{realTokenLink:function(){return v.realTokenLink}}),n.o(v,"renderElement")&&n.d(t,{renderElement:function(){return v.renderElement}}),n.o(v,"researchLink")&&n.d(t,{researchLink:function(){return v.researchLink}}),n.o(v,"reviewGraphDataLink")&&n.d(t,{reviewGraphDataLink:function(){return v.reviewGraphDataLink}}),n.o(v,"salesGraphDataLink")&&n.d(t,{salesGraphDataLink:function(){return v.salesGraphDataLink}}),n.o(v,"salesLink")&&n.d(t,{salesLink:function(){return v.salesLink}}),n.o(v,"searchGoogleGraphDataLink")&&n.d(t,{searchGoogleGraphDataLink:function(){return v.searchGoogleGraphDataLink}}),n.o(v,"signupLink")&&n.d(t,{signupLink:function(){return v.signupLink}}),n.o(v,"toggleExtension")&&n.d(t,{toggleExtension:function(){return v.toggleExtension}}),n.o(v,"tokenConnectionLink")&&n.d(t,{tokenConnectionLink:function(){return v.tokenConnectionLink}}),n.o(v,"transReactQueryStatus")&&n.d(t,{transReactQueryStatus:function(){return v.transReactQueryStatus}}),n.o(v,"transformIntoCNLink")&&n.d(t,{transformIntoCNLink:function(){return v.transformIntoCNLink}}),n.o(v,"upgradePlanLink")&&n.d(t,{upgradePlanLink:function(){return v.upgradePlanLink}}),n.o(v,"upgradeWithCouponPlanLinkV1")&&n.d(t,{upgradeWithCouponPlanLinkV1:function(){return v.upgradeWithCouponPlanLinkV1}}),n.o(v,"upsellDataLink")&&n.d(t,{upsellDataLink:function(){return v.upsellDataLink}}),n.o(v,"useEntitlements")&&n.d(t,{useEntitlements:function(){return v.useEntitlements}}),n.o(v,"useGetCacheStore")&&n.d(t,{useGetCacheStore:function(){return v.useGetCacheStore}}),n.o(v,"useGetStore")&&n.d(t,{useGetStore:function(){return v.useGetStore}}),n.o(v,"useModalState")&&n.d(t,{useModalState:function(){return v.useModalState}}),n.o(v,"useShowSpinner")&&n.d(t,{useShowSpinner:function(){return v.useShowSpinner}}),n.o(v,"useTranslation")&&n.d(t,{useTranslation:function(){return v.useTranslation}}),n.o(v,"waitForProduct")&&n.d(t,{waitForProduct:function(){return v.waitForProduct}}),n.o(v,"waitForSalesData")&&n.d(t,{waitForSalesData:function(){return v.waitForSalesData}}),n.o(v,"xrayInvocationLink")&&n.d(t,{xrayInvocationLink:function(){return v.xrayInvocationLink}});var y=n(74633);n.o(y,"BSR_ERROR_MESSAGE_MAP")&&n.d(t,{BSR_ERROR_MESSAGE_MAP:function(){return y.BSR_ERROR_MESSAGE_MAP}}),n.o(y,"FadeAnimation")&&n.d(t,{FadeAnimation:function(){return y.FadeAnimation}}),n.o(y,"MenuAsinGrabberButton")&&n.d(t,{MenuAsinGrabberButton:function(){return y.MenuAsinGrabberButton}}),n.o(y,"MenuCalculatorButton")&&n.d(t,{MenuCalculatorButton:function(){return y.MenuCalculatorButton}}),n.o(y,"MenuCalculatorWalmartButton")&&n.d(t,{MenuCalculatorWalmartButton:function(){return y.MenuCalculatorWalmartButton}}),n.o(y,"MenuFeedbackButton")&&n.d(t,{MenuFeedbackButton:function(){return y.MenuFeedbackButton}}),n.o(y,"MenuIconWrapper")&&n.d(t,{MenuIconWrapper:function(){return y.MenuIconWrapper}}),n.o(y,"MenuInventoryButton")&&n.d(t,{MenuInventoryButton:function(){return y.MenuInventoryButton}}),n.o(y,"MenuTiktokReviewInsightsButton")&&n.d(t,{MenuTiktokReviewInsightsButton:function(){return y.MenuTiktokReviewInsightsButton}}),n.o(y,"MenuXrayButton")&&n.d(t,{MenuXrayButton:function(){return y.MenuXrayButton}}),n.o(y,"MenuXrayKeywordsButton")&&n.d(t,{MenuXrayKeywordsButton:function(){return y.MenuXrayKeywordsButton}}),n.o(y,"MenuXrayWalmartButton")&&n.d(t,{MenuXrayWalmartButton:function(){return y.MenuXrayWalmartButton}}),n.o(y,"Modal")&&n.d(t,{Modal:function(){return y.Modal}}),n.o(y,"ModalWindowRoot")&&n.d(t,{ModalWindowRoot:function(){return y.ModalWindowRoot}}),n.o(y,"RootElement")&&n.d(t,{RootElement:function(){return y.RootElement}}),n.o(y,"ToastElement")&&n.d(t,{ToastElement:function(){return y.ToastElement}}),n.o(y,"TokenConnection")&&n.d(t,{TokenConnection:function(){return y.TokenConnection}}),n.o(y,"abaDataLink")&&n.d(t,{abaDataLink:function(){return y.abaDataLink}}),n.o(y,"addModalState")&&n.d(t,{addModalState:function(){return y.addModalState}}),n.o(y,"amazonLink")&&n.d(t,{amazonLink:function(){return y.amazonLink}}),n.o(y,"amazonUserSettingLink")&&n.d(t,{amazonUserSettingLink:function(){return y.amazonUserSettingLink}}),n.o(y,"bsrGraphDataLink")&&n.d(t,{bsrGraphDataLink:function(){return y.bsrGraphDataLink}}),n.o(y,"bsrGraphsSelector")&&n.d(t,{bsrGraphsSelector:function(){return y.bsrGraphsSelector}}),n.o(y,"checkIfBelowPlatinum")&&n.d(t,{checkIfBelowPlatinum:function(){return y.checkIfBelowPlatinum}}),n.o(y,"chromeStorageLocalPersister")&&n.d(t,{chromeStorageLocalPersister:function(){return y.chromeStorageLocalPersister}}),n.o(y,"controlCenterLink")&&n.d(t,{controlCenterLink:function(){return y.controlCenterLink}}),n.o(y,"dayMilliseconds")&&n.d(t,{dayMilliseconds:function(){return y.dayMilliseconds}}),n.o(y,"feedbackFormLink")&&n.d(t,{feedbackFormLink:function(){return y.feedbackFormLink}}),n.o(y,"feedbackSurvey")&&n.d(t,{feedbackSurvey:function(){return y.feedbackSurvey}}),n.o(y,"findCreateElement")&&n.d(t,{findCreateElement:function(){return y.findCreateElement}}),n.o(y,"formatPrice")&&n.d(t,{formatPrice:function(){return y.formatPrice}}),n.o(y,"get30DayRevenueByParent")&&n.d(t,{get30DayRevenueByParent:function(){return y.get30DayRevenueByParent}}),n.o(y,"get30DaySalesByParent")&&n.d(t,{get30DaySalesByParent:function(){return y.get30DaySalesByParent}}),n.o(y,"getAverageCalculations")&&n.d(t,{getAverageCalculations:function(){return y.getAverageCalculations}}),n.o(y,"getCheckLoginLink")&&n.d(t,{getCheckLoginLink:function(){return y.getCheckLoginLink}}),n.o(y,"getCommissionRate")&&n.d(t,{getCommissionRate:function(){return y.getCommissionRate}}),n.o(y,"getCountryLink")&&n.d(t,{getCountryLink:function(){return y.getCountryLink}}),n.o(y,"getCoupon")&&n.d(t,{getCoupon:function(){return y.getCoupon}}),n.o(y,"getCurrentMarketplace")&&n.d(t,{getCurrentMarketplace:function(){return y.getCurrentMarketplace}}),n.o(y,"getCurrentMarketplaceTLDomain")&&n.d(t,{getCurrentMarketplaceTLDomain:function(){return y.getCurrentMarketplaceTLDomain}}),n.o(y,"getDomain")&&n.d(t,{getDomain:function(){return y.getDomain}}),n.o(y,"getDomainFromUrl")&&n.d(t,{getDomainFromUrl:function(){return y.getDomainFromUrl}}),n.o(y,"getInfluencerVideoCount")&&n.d(t,{getInfluencerVideoCount:function(){return y.getInfluencerVideoCount}}),n.o(y,"getInterfaceLanguage")&&n.d(t,{getInterfaceLanguage:function(){return y.getInterfaceLanguage}}),n.o(y,"getInvocation")&&n.d(t,{getInvocation:function(){return y.getInvocation}}),n.o(y,"getLanguages")&&n.d(t,{getLanguages:function(){return y.getLanguages}}),n.o(y,"getMarketDataFromDomain")&&n.d(t,{getMarketDataFromDomain:function(){return y.getMarketDataFromDomain}}),n.o(y,"getMarketplaceCurrency")&&n.d(t,{getMarketplaceCurrency:function(){return y.getMarketplaceCurrency}}),n.o(y,"getModalState")&&n.d(t,{getModalState:function(){return y.getModalState}}),n.o(y,"getMyProductIdeaList")&&n.d(t,{getMyProductIdeaList:function(){return y.getMyProductIdeaList}}),n.o(y,"getProducts")&&n.d(t,{getProducts:function(){return y.getProducts}}),n.o(y,"getSalesGraph")&&n.d(t,{getSalesGraph:function(){return y.getSalesGraph}}),n.o(y,"getSellersCountries")&&n.d(t,{getSellersCountries:function(){return y.getSellersCountries}}),n.o(y,"getTiktokReviewChargeUseLink")&&n.d(t,{getTiktokReviewChargeUseLink:function(){return y.getTiktokReviewChargeUseLink}}),n.o(y,"getTranslation")&&n.d(t,{getTranslation:function(){return y.getTranslation}}),n.o(y,"getUpgradeWithCouponPlanLinkV1")&&n.d(t,{getUpgradeWithCouponPlanLinkV1:function(){return y.getUpgradeWithCouponPlanLinkV1}}),n.o(y,"getUpgradeWithCouponPlanLinkV2")&&n.d(t,{getUpgradeWithCouponPlanLinkV2:function(){return y.getUpgradeWithCouponPlanLinkV2}}),n.o(y,"graphsStore")&&n.d(t,{graphsStore:function(){return y.graphsStore}}),n.o(y,"heliumExtensionInStore")&&n.d(t,{heliumExtensionInStore:function(){return y.heliumExtensionInStore}}),n.o(y,"isPositiveValue")&&n.d(t,{isPositiveValue:function(){return y.isPositiveValue}}),n.o(y,"isReadyStatus")&&n.d(t,{isReadyStatus:function(){return y.isReadyStatus}}),n.o(y,"isWalmartProductPageNew")&&n.d(t,{isWalmartProductPageNew:function(){return y.isWalmartProductPageNew}}),n.o(y,"isWalmartSearchPage")&&n.d(t,{isWalmartSearchPage:function(){return y.isWalmartSearchPage}}),n.o(y,"learnHowToUseLink")&&n.d(t,{learnHowToUseLink:function(){return y.learnHowToUseLink}}),n.o(y,"mainHeliumLink")&&n.d(t,{mainHeliumLink:function(){return y.mainHeliumLink}}),n.o(y,"membersLink")&&n.d(t,{membersLink:function(){return y.membersLink}}),n.o(y,"parserConfigLink")&&n.d(t,{parserConfigLink:function(){return y.parserConfigLink}}),n.o(y,"priceFromApi")&&n.d(t,{priceFromApi:function(){return y.priceFromApi}}),n.o(y,"queryClient")&&n.d(t,{queryClient:function(){return y.queryClient}}),n.o(y,"realTokenLink")&&n.d(t,{realTokenLink:function(){return y.realTokenLink}}),n.o(y,"renderElement")&&n.d(t,{renderElement:function(){return y.renderElement}}),n.o(y,"researchLink")&&n.d(t,{researchLink:function(){return y.researchLink}}),n.o(y,"reviewGraphDataLink")&&n.d(t,{reviewGraphDataLink:function(){return y.reviewGraphDataLink}}),n.o(y,"salesGraphDataLink")&&n.d(t,{salesGraphDataLink:function(){return y.salesGraphDataLink}}),n.o(y,"salesLink")&&n.d(t,{salesLink:function(){return y.salesLink}}),n.o(y,"searchGoogleGraphDataLink")&&n.d(t,{searchGoogleGraphDataLink:function(){return y.searchGoogleGraphDataLink}}),n.o(y,"signupLink")&&n.d(t,{signupLink:function(){return y.signupLink}}),n.o(y,"toggleExtension")&&n.d(t,{toggleExtension:function(){return y.toggleExtension}}),n.o(y,"tokenConnectionLink")&&n.d(t,{tokenConnectionLink:function(){return y.tokenConnectionLink}}),n.o(y,"transReactQueryStatus")&&n.d(t,{transReactQueryStatus:function(){return y.transReactQueryStatus}}),n.o(y,"transformIntoCNLink")&&n.d(t,{transformIntoCNLink:function(){return y.transformIntoCNLink}}),n.o(y,"upgradePlanLink")&&n.d(t,{upgradePlanLink:function(){return y.upgradePlanLink}}),n.o(y,"upgradeWithCouponPlanLinkV1")&&n.d(t,{upgradeWithCouponPlanLinkV1:function(){return y.upgradeWithCouponPlanLinkV1}}),n.o(y,"upsellDataLink")&&n.d(t,{upsellDataLink:function(){return y.upsellDataLink}}),n.o(y,"useEntitlements")&&n.d(t,{useEntitlements:function(){return y.useEntitlements}}),n.o(y,"useGetCacheStore")&&n.d(t,{useGetCacheStore:function(){return y.useGetCacheStore}}),n.o(y,"useGetStore")&&n.d(t,{useGetStore:function(){return y.useGetStore}}),n.o(y,"useModalState")&&n.d(t,{useModalState:function(){return y.useModalState}}),n.o(y,"useShowSpinner")&&n.d(t,{useShowSpinner:function(){return y.useShowSpinner}}),n.o(y,"useTranslation")&&n.d(t,{useTranslation:function(){return y.useTranslation}}),n.o(y,"waitForProduct")&&n.d(t,{waitForProduct:function(){return y.waitForProduct}}),n.o(y,"waitForSalesData")&&n.d(t,{waitForSalesData:function(){return y.waitForSalesData}}),n.o(y,"xrayInvocationLink")&&n.d(t,{xrayInvocationLink:function(){return y.xrayInvocationLink}});var k=n(59624);n.o(k,"BSR_ERROR_MESSAGE_MAP")&&n.d(t,{BSR_ERROR_MESSAGE_MAP:function(){return k.BSR_ERROR_MESSAGE_MAP}}),n.o(k,"FadeAnimation")&&n.d(t,{FadeAnimation:function(){return k.FadeAnimation}}),n.o(k,"MenuAsinGrabberButton")&&n.d(t,{MenuAsinGrabberButton:function(){return k.MenuAsinGrabberButton}}),n.o(k,"MenuCalculatorButton")&&n.d(t,{MenuCalculatorButton:function(){return k.MenuCalculatorButton}}),n.o(k,"MenuCalculatorWalmartButton")&&n.d(t,{MenuCalculatorWalmartButton:function(){return k.MenuCalculatorWalmartButton}}),n.o(k,"MenuFeedbackButton")&&n.d(t,{MenuFeedbackButton:function(){return k.MenuFeedbackButton}}),n.o(k,"MenuIconWrapper")&&n.d(t,{MenuIconWrapper:function(){return k.MenuIconWrapper}}),n.o(k,"MenuInventoryButton")&&n.d(t,{MenuInventoryButton:function(){return k.MenuInventoryButton}}),n.o(k,"MenuTiktokReviewInsightsButton")&&n.d(t,{MenuTiktokReviewInsightsButton:function(){return k.MenuTiktokReviewInsightsButton}}),n.o(k,"MenuXrayButton")&&n.d(t,{MenuXrayButton:function(){return k.MenuXrayButton}}),n.o(k,"MenuXrayKeywordsButton")&&n.d(t,{MenuXrayKeywordsButton:function(){return k.MenuXrayKeywordsButton}}),n.o(k,"MenuXrayWalmartButton")&&n.d(t,{MenuXrayWalmartButton:function(){return k.MenuXrayWalmartButton}}),n.o(k,"Modal")&&n.d(t,{Modal:function(){return k.Modal}}),n.o(k,"ModalWindowRoot")&&n.d(t,{ModalWindowRoot:function(){return k.ModalWindowRoot}}),n.o(k,"RootElement")&&n.d(t,{RootElement:function(){return k.RootElement}}),n.o(k,"ToastElement")&&n.d(t,{ToastElement:function(){return k.ToastElement}}),n.o(k,"TokenConnection")&&n.d(t,{TokenConnection:function(){return k.TokenConnection}}),n.o(k,"abaDataLink")&&n.d(t,{abaDataLink:function(){return k.abaDataLink}}),n.o(k,"addModalState")&&n.d(t,{addModalState:function(){return k.addModalState}}),n.o(k,"amazonLink")&&n.d(t,{amazonLink:function(){return k.amazonLink}}),n.o(k,"amazonUserSettingLink")&&n.d(t,{amazonUserSettingLink:function(){return k.amazonUserSettingLink}}),n.o(k,"bsrGraphDataLink")&&n.d(t,{bsrGraphDataLink:function(){return k.bsrGraphDataLink}}),n.o(k,"bsrGraphsSelector")&&n.d(t,{bsrGraphsSelector:function(){return k.bsrGraphsSelector}}),n.o(k,"checkIfBelowPlatinum")&&n.d(t,{checkIfBelowPlatinum:function(){return k.checkIfBelowPlatinum}}),n.o(k,"chromeStorageLocalPersister")&&n.d(t,{chromeStorageLocalPersister:function(){return k.chromeStorageLocalPersister}}),n.o(k,"controlCenterLink")&&n.d(t,{controlCenterLink:function(){return k.controlCenterLink}}),n.o(k,"dayMilliseconds")&&n.d(t,{dayMilliseconds:function(){return k.dayMilliseconds}}),n.o(k,"feedbackFormLink")&&n.d(t,{feedbackFormLink:function(){return k.feedbackFormLink}}),n.o(k,"feedbackSurvey")&&n.d(t,{feedbackSurvey:function(){return k.feedbackSurvey}}),n.o(k,"findCreateElement")&&n.d(t,{findCreateElement:function(){return k.findCreateElement}}),n.o(k,"formatPrice")&&n.d(t,{formatPrice:function(){return k.formatPrice}}),n.o(k,"get30DayRevenueByParent")&&n.d(t,{get30DayRevenueByParent:function(){return k.get30DayRevenueByParent}}),n.o(k,"get30DaySalesByParent")&&n.d(t,{get30DaySalesByParent:function(){return k.get30DaySalesByParent}}),n.o(k,"getAverageCalculations")&&n.d(t,{getAverageCalculations:function(){return k.getAverageCalculations}}),n.o(k,"getCheckLoginLink")&&n.d(t,{getCheckLoginLink:function(){return k.getCheckLoginLink}}),n.o(k,"getCommissionRate")&&n.d(t,{getCommissionRate:function(){return k.getCommissionRate}}),n.o(k,"getCountryLink")&&n.d(t,{getCountryLink:function(){return k.getCountryLink}}),n.o(k,"getCoupon")&&n.d(t,{getCoupon:function(){return k.getCoupon}}),n.o(k,"getCurrentMarketplace")&&n.d(t,{getCurrentMarketplace:function(){return k.getCurrentMarketplace}}),n.o(k,"getCurrentMarketplaceTLDomain")&&n.d(t,{getCurrentMarketplaceTLDomain:function(){return k.getCurrentMarketplaceTLDomain}}),n.o(k,"getDomain")&&n.d(t,{getDomain:function(){return k.getDomain}}),n.o(k,"getDomainFromUrl")&&n.d(t,{getDomainFromUrl:function(){return k.getDomainFromUrl}}),n.o(k,"getInfluencerVideoCount")&&n.d(t,{getInfluencerVideoCount:function(){return k.getInfluencerVideoCount}}),n.o(k,"getInterfaceLanguage")&&n.d(t,{getInterfaceLanguage:function(){return k.getInterfaceLanguage}}),n.o(k,"getInvocation")&&n.d(t,{getInvocation:function(){return k.getInvocation}}),n.o(k,"getLanguages")&&n.d(t,{getLanguages:function(){return k.getLanguages}}),n.o(k,"getMarketDataFromDomain")&&n.d(t,{getMarketDataFromDomain:function(){return k.getMarketDataFromDomain}}),n.o(k,"getMarketplaceCurrency")&&n.d(t,{getMarketplaceCurrency:function(){return k.getMarketplaceCurrency}}),n.o(k,"getModalState")&&n.d(t,{getModalState:function(){return k.getModalState}}),n.o(k,"getMyProductIdeaList")&&n.d(t,{getMyProductIdeaList:function(){return k.getMyProductIdeaList}}),n.o(k,"getProducts")&&n.d(t,{getProducts:function(){return k.getProducts}}),n.o(k,"getSalesGraph")&&n.d(t,{getSalesGraph:function(){return k.getSalesGraph}}),n.o(k,"getSellersCountries")&&n.d(t,{getSellersCountries:function(){return k.getSellersCountries}}),n.o(k,"getTiktokReviewChargeUseLink")&&n.d(t,{getTiktokReviewChargeUseLink:function(){return k.getTiktokReviewChargeUseLink}}),n.o(k,"getTranslation")&&n.d(t,{getTranslation:function(){return k.getTranslation}}),n.o(k,"getUpgradeWithCouponPlanLinkV1")&&n.d(t,{getUpgradeWithCouponPlanLinkV1:function(){return k.getUpgradeWithCouponPlanLinkV1}}),n.o(k,"getUpgradeWithCouponPlanLinkV2")&&n.d(t,{getUpgradeWithCouponPlanLinkV2:function(){return k.getUpgradeWithCouponPlanLinkV2}}),n.o(k,"graphsStore")&&n.d(t,{graphsStore:function(){return k.graphsStore}}),n.o(k,"heliumExtensionInStore")&&n.d(t,{heliumExtensionInStore:function(){return k.heliumExtensionInStore}}),n.o(k,"isPositiveValue")&&n.d(t,{isPositiveValue:function(){return k.isPositiveValue}}),n.o(k,"isReadyStatus")&&n.d(t,{isReadyStatus:function(){return k.isReadyStatus}}),n.o(k,"isWalmartProductPageNew")&&n.d(t,{isWalmartProductPageNew:function(){return k.isWalmartProductPageNew}}),n.o(k,"isWalmartSearchPage")&&n.d(t,{isWalmartSearchPage:function(){return k.isWalmartSearchPage}}),n.o(k,"learnHowToUseLink")&&n.d(t,{learnHowToUseLink:function(){return k.learnHowToUseLink}}),n.o(k,"mainHeliumLink")&&n.d(t,{mainHeliumLink:function(){return k.mainHeliumLink}}),n.o(k,"membersLink")&&n.d(t,{membersLink:function(){return k.membersLink}}),n.o(k,"parserConfigLink")&&n.d(t,{parserConfigLink:function(){return k.parserConfigLink}}),n.o(k,"priceFromApi")&&n.d(t,{priceFromApi:function(){return k.priceFromApi}}),n.o(k,"queryClient")&&n.d(t,{queryClient:function(){return k.queryClient}}),n.o(k,"realTokenLink")&&n.d(t,{realTokenLink:function(){return k.realTokenLink}}),n.o(k,"renderElement")&&n.d(t,{renderElement:function(){return k.renderElement}}),n.o(k,"researchLink")&&n.d(t,{researchLink:function(){return k.researchLink}}),n.o(k,"reviewGraphDataLink")&&n.d(t,{reviewGraphDataLink:function(){return k.reviewGraphDataLink}}),n.o(k,"salesGraphDataLink")&&n.d(t,{salesGraphDataLink:function(){return k.salesGraphDataLink}}),n.o(k,"salesLink")&&n.d(t,{salesLink:function(){return k.salesLink}}),n.o(k,"searchGoogleGraphDataLink")&&n.d(t,{searchGoogleGraphDataLink:function(){return k.searchGoogleGraphDataLink}}),n.o(k,"signupLink")&&n.d(t,{signupLink:function(){return k.signupLink}}),n.o(k,"toggleExtension")&&n.d(t,{toggleExtension:function(){return k.toggleExtension}}),n.o(k,"tokenConnectionLink")&&n.d(t,{tokenConnectionLink:function(){return k.tokenConnectionLink}}),n.o(k,"transReactQueryStatus")&&n.d(t,{transReactQueryStatus:function(){return k.transReactQueryStatus}}),n.o(k,"transformIntoCNLink")&&n.d(t,{transformIntoCNLink:function(){return k.transformIntoCNLink}}),n.o(k,"upgradePlanLink")&&n.d(t,{upgradePlanLink:function(){return k.upgradePlanLink}}),n.o(k,"upgradeWithCouponPlanLinkV1")&&n.d(t,{upgradeWithCouponPlanLinkV1:function(){return k.upgradeWithCouponPlanLinkV1}}),n.o(k,"upsellDataLink")&&n.d(t,{upsellDataLink:function(){return k.upsellDataLink}}),n.o(k,"useEntitlements")&&n.d(t,{useEntitlements:function(){return k.useEntitlements}}),n.o(k,"useGetCacheStore")&&n.d(t,{useGetCacheStore:function(){return k.useGetCacheStore}}),n.o(k,"useGetStore")&&n.d(t,{useGetStore:function(){return k.useGetStore}}),n.o(k,"useModalState")&&n.d(t,{useModalState:function(){return k.useModalState}}),n.o(k,"useShowSpinner")&&n.d(t,{useShowSpinner:function(){return k.useShowSpinner}}),n.o(k,"useTranslation")&&n.d(t,{useTranslation:function(){return k.useTranslation}}),n.o(k,"waitForProduct")&&n.d(t,{waitForProduct:function(){return k.waitForProduct}}),n.o(k,"waitForSalesData")&&n.d(t,{waitForSalesData:function(){return k.waitForSalesData}}),n.o(k,"xrayInvocationLink")&&n.d(t,{xrayInvocationLink:function(){return k.xrayInvocationLink}});var x=n(18054);n.o(x,"BSR_ERROR_MESSAGE_MAP")&&n.d(t,{BSR_ERROR_MESSAGE_MAP:function(){return x.BSR_ERROR_MESSAGE_MAP}}),n.o(x,"FadeAnimation")&&n.d(t,{FadeAnimation:function(){return x.FadeAnimation}}),n.o(x,"MenuAsinGrabberButton")&&n.d(t,{MenuAsinGrabberButton:function(){return x.MenuAsinGrabberButton}}),n.o(x,"MenuCalculatorButton")&&n.d(t,{MenuCalculatorButton:function(){return x.MenuCalculatorButton}}),n.o(x,"MenuCalculatorWalmartButton")&&n.d(t,{MenuCalculatorWalmartButton:function(){return x.MenuCalculatorWalmartButton}}),n.o(x,"MenuFeedbackButton")&&n.d(t,{MenuFeedbackButton:function(){return x.MenuFeedbackButton}}),n.o(x,"MenuIconWrapper")&&n.d(t,{MenuIconWrapper:function(){return x.MenuIconWrapper}}),n.o(x,"MenuInventoryButton")&&n.d(t,{MenuInventoryButton:function(){return x.MenuInventoryButton}}),n.o(x,"MenuTiktokReviewInsightsButton")&&n.d(t,{MenuTiktokReviewInsightsButton:function(){return x.MenuTiktokReviewInsightsButton}}),n.o(x,"MenuXrayButton")&&n.d(t,{MenuXrayButton:function(){return x.MenuXrayButton}}),n.o(x,"MenuXrayKeywordsButton")&&n.d(t,{MenuXrayKeywordsButton:function(){return x.MenuXrayKeywordsButton}}),n.o(x,"MenuXrayWalmartButton")&&n.d(t,{MenuXrayWalmartButton:function(){return x.MenuXrayWalmartButton}}),n.o(x,"Modal")&&n.d(t,{Modal:function(){return x.Modal}}),n.o(x,"ModalWindowRoot")&&n.d(t,{ModalWindowRoot:function(){return x.ModalWindowRoot}}),n.o(x,"RootElement")&&n.d(t,{RootElement:function(){return x.RootElement}}),n.o(x,"ToastElement")&&n.d(t,{ToastElement:function(){return x.ToastElement}}),n.o(x,"TokenConnection")&&n.d(t,{TokenConnection:function(){return x.TokenConnection}}),n.o(x,"abaDataLink")&&n.d(t,{abaDataLink:function(){return x.abaDataLink}}),n.o(x,"addModalState")&&n.d(t,{addModalState:function(){return x.addModalState}}),n.o(x,"amazonLink")&&n.d(t,{amazonLink:function(){return x.amazonLink}}),n.o(x,"amazonUserSettingLink")&&n.d(t,{amazonUserSettingLink:function(){return x.amazonUserSettingLink}}),n.o(x,"bsrGraphDataLink")&&n.d(t,{bsrGraphDataLink:function(){return x.bsrGraphDataLink}}),n.o(x,"bsrGraphsSelector")&&n.d(t,{bsrGraphsSelector:function(){return x.bsrGraphsSelector}}),n.o(x,"checkIfBelowPlatinum")&&n.d(t,{checkIfBelowPlatinum:function(){return x.checkIfBelowPlatinum}}),n.o(x,"chromeStorageLocalPersister")&&n.d(t,{chromeStorageLocalPersister:function(){return x.chromeStorageLocalPersister}}),n.o(x,"controlCenterLink")&&n.d(t,{controlCenterLink:function(){return x.controlCenterLink}}),n.o(x,"dayMilliseconds")&&n.d(t,{dayMilliseconds:function(){return x.dayMilliseconds}}),n.o(x,"feedbackFormLink")&&n.d(t,{feedbackFormLink:function(){return x.feedbackFormLink}}),n.o(x,"feedbackSurvey")&&n.d(t,{feedbackSurvey:function(){return x.feedbackSurvey}}),n.o(x,"findCreateElement")&&n.d(t,{findCreateElement:function(){return x.findCreateElement}}),n.o(x,"formatPrice")&&n.d(t,{formatPrice:function(){return x.formatPrice}}),n.o(x,"get30DayRevenueByParent")&&n.d(t,{get30DayRevenueByParent:function(){return x.get30DayRevenueByParent}}),n.o(x,"get30DaySalesByParent")&&n.d(t,{get30DaySalesByParent:function(){return x.get30DaySalesByParent}}),n.o(x,"getAverageCalculations")&&n.d(t,{getAverageCalculations:function(){return x.getAverageCalculations}}),n.o(x,"getCheckLoginLink")&&n.d(t,{getCheckLoginLink:function(){return x.getCheckLoginLink}}),n.o(x,"getCommissionRate")&&n.d(t,{getCommissionRate:function(){return x.getCommissionRate}}),n.o(x,"getCountryLink")&&n.d(t,{getCountryLink:function(){return x.getCountryLink}}),n.o(x,"getCoupon")&&n.d(t,{getCoupon:function(){return x.getCoupon}}),n.o(x,"getCurrentMarketplace")&&n.d(t,{getCurrentMarketplace:function(){return x.getCurrentMarketplace}}),n.o(x,"getCurrentMarketplaceTLDomain")&&n.d(t,{getCurrentMarketplaceTLDomain:function(){return x.getCurrentMarketplaceTLDomain}}),n.o(x,"getDomain")&&n.d(t,{getDomain:function(){return x.getDomain}}),n.o(x,"getDomainFromUrl")&&n.d(t,{getDomainFromUrl:function(){return x.getDomainFromUrl}}),n.o(x,"getInfluencerVideoCount")&&n.d(t,{getInfluencerVideoCount:function(){return x.getInfluencerVideoCount}}),n.o(x,"getInterfaceLanguage")&&n.d(t,{getInterfaceLanguage:function(){return x.getInterfaceLanguage}}),n.o(x,"getInvocation")&&n.d(t,{getInvocation:function(){return x.getInvocation}}),n.o(x,"getLanguages")&&n.d(t,{getLanguages:function(){return x.getLanguages}}),n.o(x,"getMarketDataFromDomain")&&n.d(t,{getMarketDataFromDomain:function(){return x.getMarketDataFromDomain}}),n.o(x,"getMarketplaceCurrency")&&n.d(t,{getMarketplaceCurrency:function(){return x.getMarketplaceCurrency}}),n.o(x,"getModalState")&&n.d(t,{getModalState:function(){return x.getModalState}}),n.o(x,"getMyProductIdeaList")&&n.d(t,{getMyProductIdeaList:function(){return x.getMyProductIdeaList}}),n.o(x,"getProducts")&&n.d(t,{getProducts:function(){return x.getProducts}}),n.o(x,"getSalesGraph")&&n.d(t,{getSalesGraph:function(){return x.getSalesGraph}}),n.o(x,"getSellersCountries")&&n.d(t,{getSellersCountries:function(){return x.getSellersCountries}}),n.o(x,"getTiktokReviewChargeUseLink")&&n.d(t,{getTiktokReviewChargeUseLink:function(){return x.getTiktokReviewChargeUseLink}}),n.o(x,"getTranslation")&&n.d(t,{getTranslation:function(){return x.getTranslation}}),n.o(x,"getUpgradeWithCouponPlanLinkV1")&&n.d(t,{getUpgradeWithCouponPlanLinkV1:function(){return x.getUpgradeWithCouponPlanLinkV1}}),n.o(x,"getUpgradeWithCouponPlanLinkV2")&&n.d(t,{getUpgradeWithCouponPlanLinkV2:function(){return x.getUpgradeWithCouponPlanLinkV2}}),n.o(x,"graphsStore")&&n.d(t,{graphsStore:function(){return x.graphsStore}}),n.o(x,"heliumExtensionInStore")&&n.d(t,{heliumExtensionInStore:function(){return x.heliumExtensionInStore}}),n.o(x,"isPositiveValue")&&n.d(t,{isPositiveValue:function(){return x.isPositiveValue}}),n.o(x,"isReadyStatus")&&n.d(t,{isReadyStatus:function(){return x.isReadyStatus}}),n.o(x,"isWalmartProductPageNew")&&n.d(t,{isWalmartProductPageNew:function(){return x.isWalmartProductPageNew}}),n.o(x,"isWalmartSearchPage")&&n.d(t,{isWalmartSearchPage:function(){return x.isWalmartSearchPage}}),n.o(x,"learnHowToUseLink")&&n.d(t,{learnHowToUseLink:function(){return x.learnHowToUseLink}}),n.o(x,"mainHeliumLink")&&n.d(t,{mainHeliumLink:function(){return x.mainHeliumLink}}),n.o(x,"membersLink")&&n.d(t,{membersLink:function(){return x.membersLink}}),n.o(x,"parserConfigLink")&&n.d(t,{parserConfigLink:function(){return x.parserConfigLink}}),n.o(x,"priceFromApi")&&n.d(t,{priceFromApi:function(){return x.priceFromApi}}),n.o(x,"queryClient")&&n.d(t,{queryClient:function(){return x.queryClient}}),n.o(x,"realTokenLink")&&n.d(t,{realTokenLink:function(){return x.realTokenLink}}),n.o(x,"renderElement")&&n.d(t,{renderElement:function(){return x.renderElement}}),n.o(x,"researchLink")&&n.d(t,{researchLink:function(){return x.researchLink}}),n.o(x,"reviewGraphDataLink")&&n.d(t,{reviewGraphDataLink:function(){return x.reviewGraphDataLink}}),n.o(x,"salesGraphDataLink")&&n.d(t,{salesGraphDataLink:function(){return x.salesGraphDataLink}}),n.o(x,"salesLink")&&n.d(t,{salesLink:function(){return x.salesLink}}),n.o(x,"searchGoogleGraphDataLink")&&n.d(t,{searchGoogleGraphDataLink:function(){return x.searchGoogleGraphDataLink}}),n.o(x,"signupLink")&&n.d(t,{signupLink:function(){return x.signupLink}}),n.o(x,"toggleExtension")&&n.d(t,{toggleExtension:function(){return x.toggleExtension}}),n.o(x,"tokenConnectionLink")&&n.d(t,{tokenConnectionLink:function(){return x.tokenConnectionLink}}),n.o(x,"transReactQueryStatus")&&n.d(t,{transReactQueryStatus:function(){return x.transReactQueryStatus}}),n.o(x,"transformIntoCNLink")&&n.d(t,{transformIntoCNLink:function(){return x.transformIntoCNLink}}),n.o(x,"upgradePlanLink")&&n.d(t,{upgradePlanLink:function(){return x.upgradePlanLink}}),n.o(x,"upgradeWithCouponPlanLinkV1")&&n.d(t,{upgradeWithCouponPlanLinkV1:function(){return x.upgradeWithCouponPlanLinkV1}}),n.o(x,"upsellDataLink")&&n.d(t,{upsellDataLink:function(){return x.upsellDataLink}}),n.o(x,"useEntitlements")&&n.d(t,{useEntitlements:function(){return x.useEntitlements}}),n.o(x,"useGetCacheStore")&&n.d(t,{useGetCacheStore:function(){return x.useGetCacheStore}}),n.o(x,"useGetStore")&&n.d(t,{useGetStore:function(){return x.useGetStore}}),n.o(x,"useModalState")&&n.d(t,{useModalState:function(){return x.useModalState}}),n.o(x,"useShowSpinner")&&n.d(t,{useShowSpinner:function(){return x.useShowSpinner}}),n.o(x,"useTranslation")&&n.d(t,{useTranslation:function(){return x.useTranslation}}),n.o(x,"waitForProduct")&&n.d(t,{waitForProduct:function(){return x.waitForProduct}}),n.o(x,"waitForSalesData")&&n.d(t,{waitForSalesData:function(){return x.waitForSalesData}}),n.o(x,"xrayInvocationLink")&&n.d(t,{xrayInvocationLink:function(){return x.xrayInvocationLink}});n(8394);var w=n(56827);n.o(w,"BSR_ERROR_MESSAGE_MAP")&&n.d(t,{BSR_ERROR_MESSAGE_MAP:function(){return w.BSR_ERROR_MESSAGE_MAP}}),n.o(w,"FadeAnimation")&&n.d(t,{FadeAnimation:function(){return w.FadeAnimation}}),n.o(w,"MenuAsinGrabberButton")&&n.d(t,{MenuAsinGrabberButton:function(){return w.MenuAsinGrabberButton}}),n.o(w,"MenuCalculatorButton")&&n.d(t,{MenuCalculatorButton:function(){return w.MenuCalculatorButton}}),n.o(w,"MenuCalculatorWalmartButton")&&n.d(t,{MenuCalculatorWalmartButton:function(){return w.MenuCalculatorWalmartButton}}),n.o(w,"MenuFeedbackButton")&&n.d(t,{MenuFeedbackButton:function(){return w.MenuFeedbackButton}}),n.o(w,"MenuIconWrapper")&&n.d(t,{MenuIconWrapper:function(){return w.MenuIconWrapper}}),n.o(w,"MenuInventoryButton")&&n.d(t,{MenuInventoryButton:function(){return w.MenuInventoryButton}}),n.o(w,"MenuTiktokReviewInsightsButton")&&n.d(t,{MenuTiktokReviewInsightsButton:function(){return w.MenuTiktokReviewInsightsButton}}),n.o(w,"MenuXrayButton")&&n.d(t,{MenuXrayButton:function(){return w.MenuXrayButton}}),n.o(w,"MenuXrayKeywordsButton")&&n.d(t,{MenuXrayKeywordsButton:function(){return w.MenuXrayKeywordsButton}}),n.o(w,"MenuXrayWalmartButton")&&n.d(t,{MenuXrayWalmartButton:function(){return w.MenuXrayWalmartButton}}),n.o(w,"Modal")&&n.d(t,{Modal:function(){return w.Modal}}),n.o(w,"ModalWindowRoot")&&n.d(t,{ModalWindowRoot:function(){return w.ModalWindowRoot}}),n.o(w,"RootElement")&&n.d(t,{RootElement:function(){return w.RootElement}}),n.o(w,"ToastElement")&&n.d(t,{ToastElement:function(){return w.ToastElement}}),n.o(w,"TokenConnection")&&n.d(t,{TokenConnection:function(){return w.TokenConnection}}),n.o(w,"abaDataLink")&&n.d(t,{abaDataLink:function(){return w.abaDataLink}}),n.o(w,"addModalState")&&n.d(t,{addModalState:function(){return w.addModalState}}),n.o(w,"amazonLink")&&n.d(t,{amazonLink:function(){return w.amazonLink}}),n.o(w,"amazonUserSettingLink")&&n.d(t,{amazonUserSettingLink:function(){return w.amazonUserSettingLink}}),n.o(w,"bsrGraphDataLink")&&n.d(t,{bsrGraphDataLink:function(){return w.bsrGraphDataLink}}),n.o(w,"bsrGraphsSelector")&&n.d(t,{bsrGraphsSelector:function(){return w.bsrGraphsSelector}}),n.o(w,"checkIfBelowPlatinum")&&n.d(t,{checkIfBelowPlatinum:function(){return w.checkIfBelowPlatinum}}),n.o(w,"chromeStorageLocalPersister")&&n.d(t,{chromeStorageLocalPersister:function(){return w.chromeStorageLocalPersister}}),n.o(w,"controlCenterLink")&&n.d(t,{controlCenterLink:function(){return w.controlCenterLink}}),n.o(w,"dayMilliseconds")&&n.d(t,{dayMilliseconds:function(){return w.dayMilliseconds}}),n.o(w,"feedbackFormLink")&&n.d(t,{feedbackFormLink:function(){return w.feedbackFormLink}}),n.o(w,"feedbackSurvey")&&n.d(t,{feedbackSurvey:function(){return w.feedbackSurvey}}),n.o(w,"findCreateElement")&&n.d(t,{findCreateElement:function(){return w.findCreateElement}}),n.o(w,"formatPrice")&&n.d(t,{formatPrice:function(){return w.formatPrice}}),n.o(w,"get30DayRevenueByParent")&&n.d(t,{get30DayRevenueByParent:function(){return w.get30DayRevenueByParent}}),n.o(w,"get30DaySalesByParent")&&n.d(t,{get30DaySalesByParent:function(){return w.get30DaySalesByParent}}),n.o(w,"getAverageCalculations")&&n.d(t,{getAverageCalculations:function(){return w.getAverageCalculations}}),n.o(w,"getCheckLoginLink")&&n.d(t,{getCheckLoginLink:function(){return w.getCheckLoginLink}}),n.o(w,"getCommissionRate")&&n.d(t,{getCommissionRate:function(){return w.getCommissionRate}}),n.o(w,"getCountryLink")&&n.d(t,{getCountryLink:function(){return w.getCountryLink}}),n.o(w,"getCoupon")&&n.d(t,{getCoupon:function(){return w.getCoupon}}),n.o(w,"getCurrentMarketplace")&&n.d(t,{getCurrentMarketplace:function(){return w.getCurrentMarketplace}}),n.o(w,"getCurrentMarketplaceTLDomain")&&n.d(t,{getCurrentMarketplaceTLDomain:function(){return w.getCurrentMarketplaceTLDomain}}),n.o(w,"getDomain")&&n.d(t,{getDomain:function(){return w.getDomain}}),n.o(w,"getDomainFromUrl")&&n.d(t,{getDomainFromUrl:function(){return w.getDomainFromUrl}}),n.o(w,"getInfluencerVideoCount")&&n.d(t,{getInfluencerVideoCount:function(){return w.getInfluencerVideoCount}}),n.o(w,"getInterfaceLanguage")&&n.d(t,{getInterfaceLanguage:function(){return w.getInterfaceLanguage}}),n.o(w,"getInvocation")&&n.d(t,{getInvocation:function(){return w.getInvocation}}),n.o(w,"getLanguages")&&n.d(t,{getLanguages:function(){return w.getLanguages}}),n.o(w,"getMarketDataFromDomain")&&n.d(t,{getMarketDataFromDomain:function(){return w.getMarketDataFromDomain}}),n.o(w,"getMarketplaceCurrency")&&n.d(t,{getMarketplaceCurrency:function(){return w.getMarketplaceCurrency}}),n.o(w,"getModalState")&&n.d(t,{getModalState:function(){return w.getModalState}}),n.o(w,"getMyProductIdeaList")&&n.d(t,{getMyProductIdeaList:function(){return w.getMyProductIdeaList}}),n.o(w,"getProducts")&&n.d(t,{getProducts:function(){return w.getProducts}}),n.o(w,"getSalesGraph")&&n.d(t,{getSalesGraph:function(){return w.getSalesGraph}}),n.o(w,"getSellersCountries")&&n.d(t,{getSellersCountries:function(){return w.getSellersCountries}}),n.o(w,"getTiktokReviewChargeUseLink")&&n.d(t,{getTiktokReviewChargeUseLink:function(){return w.getTiktokReviewChargeUseLink}}),n.o(w,"getTranslation")&&n.d(t,{getTranslation:function(){return w.getTranslation}}),n.o(w,"getUpgradeWithCouponPlanLinkV1")&&n.d(t,{getUpgradeWithCouponPlanLinkV1:function(){return w.getUpgradeWithCouponPlanLinkV1}}),n.o(w,"getUpgradeWithCouponPlanLinkV2")&&n.d(t,{getUpgradeWithCouponPlanLinkV2:function(){return w.getUpgradeWithCouponPlanLinkV2}}),n.o(w,"graphsStore")&&n.d(t,{graphsStore:function(){return w.graphsStore}}),n.o(w,"heliumExtensionInStore")&&n.d(t,{heliumExtensionInStore:function(){return w.heliumExtensionInStore}}),n.o(w,"isPositiveValue")&&n.d(t,{isPositiveValue:function(){return w.isPositiveValue}}),n.o(w,"isReadyStatus")&&n.d(t,{isReadyStatus:function(){return w.isReadyStatus}}),n.o(w,"isWalmartProductPageNew")&&n.d(t,{isWalmartProductPageNew:function(){return w.isWalmartProductPageNew}}),n.o(w,"isWalmartSearchPage")&&n.d(t,{isWalmartSearchPage:function(){return w.isWalmartSearchPage}}),n.o(w,"learnHowToUseLink")&&n.d(t,{learnHowToUseLink:function(){return w.learnHowToUseLink}}),n.o(w,"mainHeliumLink")&&n.d(t,{mainHeliumLink:function(){return w.mainHeliumLink}}),n.o(w,"membersLink")&&n.d(t,{membersLink:function(){return w.membersLink}}),n.o(w,"parserConfigLink")&&n.d(t,{parserConfigLink:function(){return w.parserConfigLink}}),n.o(w,"priceFromApi")&&n.d(t,{priceFromApi:function(){return w.priceFromApi}}),n.o(w,"queryClient")&&n.d(t,{queryClient:function(){return w.queryClient}}),n.o(w,"realTokenLink")&&n.d(t,{realTokenLink:function(){return w.realTokenLink}}),n.o(w,"renderElement")&&n.d(t,{renderElement:function(){return w.renderElement}}),n.o(w,"researchLink")&&n.d(t,{researchLink:function(){return w.researchLink}}),n.o(w,"reviewGraphDataLink")&&n.d(t,{reviewGraphDataLink:function(){return w.reviewGraphDataLink}}),n.o(w,"salesGraphDataLink")&&n.d(t,{salesGraphDataLink:function(){return w.salesGraphDataLink}}),n.o(w,"salesLink")&&n.d(t,{salesLink:function(){return w.salesLink}}),n.o(w,"searchGoogleGraphDataLink")&&n.d(t,{searchGoogleGraphDataLink:function(){return w.searchGoogleGraphDataLink}}),n.o(w,"signupLink")&&n.d(t,{signupLink:function(){return w.signupLink}}),n.o(w,"toggleExtension")&&n.d(t,{toggleExtension:function(){return w.toggleExtension}}),n.o(w,"tokenConnectionLink")&&n.d(t,{tokenConnectionLink:function(){return w.tokenConnectionLink}}),n.o(w,"transReactQueryStatus")&&n.d(t,{transReactQueryStatus:function(){return w.transReactQueryStatus}}),n.o(w,"transformIntoCNLink")&&n.d(t,{transformIntoCNLink:function(){return w.transformIntoCNLink}}),n.o(w,"upgradePlanLink")&&n.d(t,{upgradePlanLink:function(){return w.upgradePlanLink}}),n.o(w,"upgradeWithCouponPlanLinkV1")&&n.d(t,{upgradeWithCouponPlanLinkV1:function(){return w.upgradeWithCouponPlanLinkV1}}),n.o(w,"upsellDataLink")&&n.d(t,{upsellDataLink:function(){return w.upsellDataLink}}),n.o(w,"useEntitlements")&&n.d(t,{useEntitlements:function(){return w.useEntitlements}}),n.o(w,"useGetCacheStore")&&n.d(t,{useGetCacheStore:function(){return w.useGetCacheStore}}),n.o(w,"useGetStore")&&n.d(t,{useGetStore:function(){return w.useGetStore}}),n.o(w,"useModalState")&&n.d(t,{useModalState:function(){return w.useModalState}}),n.o(w,"useShowSpinner")&&n.d(t,{useShowSpinner:function(){return w.useShowSpinner}}),n.o(w,"useTranslation")&&n.d(t,{useTranslation:function(){return w.useTranslation}}),n.o(w,"waitForProduct")&&n.d(t,{waitForProduct:function(){return w.waitForProduct}}),n.o(w,"waitForSalesData")&&n.d(t,{waitForSalesData:function(){return w.waitForSalesData}}),n.o(w,"xrayInvocationLink")&&n.d(t,{xrayInvocationLink:function(){return w.xrayInvocationLink}});var b=n(11469);n.o(b,"BSR_ERROR_MESSAGE_MAP")&&n.d(t,{BSR_ERROR_MESSAGE_MAP:function(){return b.BSR_ERROR_MESSAGE_MAP}}),n.o(b,"FadeAnimation")&&n.d(t,{FadeAnimation:function(){return b.FadeAnimation}}),n.o(b,"MenuAsinGrabberButton")&&n.d(t,{MenuAsinGrabberButton:function(){return b.MenuAsinGrabberButton}}),n.o(b,"MenuCalculatorButton")&&n.d(t,{MenuCalculatorButton:function(){return b.MenuCalculatorButton}}),n.o(b,"MenuCalculatorWalmartButton")&&n.d(t,{MenuCalculatorWalmartButton:function(){return b.MenuCalculatorWalmartButton}}),n.o(b,"MenuFeedbackButton")&&n.d(t,{MenuFeedbackButton:function(){return b.MenuFeedbackButton}}),n.o(b,"MenuIconWrapper")&&n.d(t,{MenuIconWrapper:function(){return b.MenuIconWrapper}}),n.o(b,"MenuInventoryButton")&&n.d(t,{MenuInventoryButton:function(){return b.MenuInventoryButton}}),n.o(b,"MenuTiktokReviewInsightsButton")&&n.d(t,{MenuTiktokReviewInsightsButton:function(){return b.MenuTiktokReviewInsightsButton}}),n.o(b,"MenuXrayButton")&&n.d(t,{MenuXrayButton:function(){return b.MenuXrayButton}}),n.o(b,"MenuXrayKeywordsButton")&&n.d(t,{MenuXrayKeywordsButton:function(){return b.MenuXrayKeywordsButton}}),n.o(b,"MenuXrayWalmartButton")&&n.d(t,{MenuXrayWalmartButton:function(){return b.MenuXrayWalmartButton}}),n.o(b,"Modal")&&n.d(t,{Modal:function(){return b.Modal}}),n.o(b,"ModalWindowRoot")&&n.d(t,{ModalWindowRoot:function(){return b.ModalWindowRoot}}),n.o(b,"RootElement")&&n.d(t,{RootElement:function(){return b.RootElement}}),n.o(b,"ToastElement")&&n.d(t,{ToastElement:function(){return b.ToastElement}}),n.o(b,"TokenConnection")&&n.d(t,{TokenConnection:function(){return b.TokenConnection}}),n.o(b,"abaDataLink")&&n.d(t,{abaDataLink:function(){return b.abaDataLink}}),n.o(b,"addModalState")&&n.d(t,{addModalState:function(){return b.addModalState}}),n.o(b,"amazonLink")&&n.d(t,{amazonLink:function(){return b.amazonLink}}),n.o(b,"amazonUserSettingLink")&&n.d(t,{amazonUserSettingLink:function(){return b.amazonUserSettingLink}}),n.o(b,"bsrGraphDataLink")&&n.d(t,{bsrGraphDataLink:function(){return b.bsrGraphDataLink}}),n.o(b,"bsrGraphsSelector")&&n.d(t,{bsrGraphsSelector:function(){return b.bsrGraphsSelector}}),n.o(b,"checkIfBelowPlatinum")&&n.d(t,{checkIfBelowPlatinum:function(){return b.checkIfBelowPlatinum}}),n.o(b,"chromeStorageLocalPersister")&&n.d(t,{chromeStorageLocalPersister:function(){return b.chromeStorageLocalPersister}}),n.o(b,"controlCenterLink")&&n.d(t,{controlCenterLink:function(){return b.controlCenterLink}}),n.o(b,"dayMilliseconds")&&n.d(t,{dayMilliseconds:function(){return b.dayMilliseconds}}),n.o(b,"feedbackFormLink")&&n.d(t,{feedbackFormLink:function(){return b.feedbackFormLink}}),n.o(b,"feedbackSurvey")&&n.d(t,{feedbackSurvey:function(){return b.feedbackSurvey}}),n.o(b,"findCreateElement")&&n.d(t,{findCreateElement:function(){return b.findCreateElement}}),n.o(b,"formatPrice")&&n.d(t,{formatPrice:function(){return b.formatPrice}}),n.o(b,"get30DayRevenueByParent")&&n.d(t,{get30DayRevenueByParent:function(){return b.get30DayRevenueByParent}}),n.o(b,"get30DaySalesByParent")&&n.d(t,{get30DaySalesByParent:function(){return b.get30DaySalesByParent}}),n.o(b,"getAverageCalculations")&&n.d(t,{getAverageCalculations:function(){return b.getAverageCalculations}}),n.o(b,"getCheckLoginLink")&&n.d(t,{getCheckLoginLink:function(){return b.getCheckLoginLink}}),n.o(b,"getCommissionRate")&&n.d(t,{getCommissionRate:function(){return b.getCommissionRate}}),n.o(b,"getCountryLink")&&n.d(t,{getCountryLink:function(){return b.getCountryLink}}),n.o(b,"getCoupon")&&n.d(t,{getCoupon:function(){return b.getCoupon}}),n.o(b,"getCurrentMarketplace")&&n.d(t,{getCurrentMarketplace:function(){return b.getCurrentMarketplace}}),n.o(b,"getCurrentMarketplaceTLDomain")&&n.d(t,{getCurrentMarketplaceTLDomain:function(){return b.getCurrentMarketplaceTLDomain}}),n.o(b,"getDomain")&&n.d(t,{getDomain:function(){return b.getDomain}}),n.o(b,"getDomainFromUrl")&&n.d(t,{getDomainFromUrl:function(){return b.getDomainFromUrl}}),n.o(b,"getInfluencerVideoCount")&&n.d(t,{getInfluencerVideoCount:function(){return b.getInfluencerVideoCount}}),n.o(b,"getInterfaceLanguage")&&n.d(t,{getInterfaceLanguage:function(){return b.getInterfaceLanguage}}),n.o(b,"getInvocation")&&n.d(t,{getInvocation:function(){return b.getInvocation}}),n.o(b,"getLanguages")&&n.d(t,{getLanguages:function(){return b.getLanguages}}),n.o(b,"getMarketDataFromDomain")&&n.d(t,{getMarketDataFromDomain:function(){return b.getMarketDataFromDomain}}),n.o(b,"getMarketplaceCurrency")&&n.d(t,{getMarketplaceCurrency:function(){return b.getMarketplaceCurrency}}),n.o(b,"getModalState")&&n.d(t,{getModalState:function(){return b.getModalState}}),n.o(b,"getMyProductIdeaList")&&n.d(t,{getMyProductIdeaList:function(){return b.getMyProductIdeaList}}),n.o(b,"getProducts")&&n.d(t,{getProducts:function(){return b.getProducts}}),n.o(b,"getSalesGraph")&&n.d(t,{getSalesGraph:function(){return b.getSalesGraph}}),n.o(b,"getSellersCountries")&&n.d(t,{getSellersCountries:function(){return b.getSellersCountries}}),n.o(b,"getTiktokReviewChargeUseLink")&&n.d(t,{getTiktokReviewChargeUseLink:function(){return b.getTiktokReviewChargeUseLink}}),n.o(b,"getTranslation")&&n.d(t,{getTranslation:function(){return b.getTranslation}}),n.o(b,"getUpgradeWithCouponPlanLinkV1")&&n.d(t,{getUpgradeWithCouponPlanLinkV1:function(){return b.getUpgradeWithCouponPlanLinkV1}}),n.o(b,"getUpgradeWithCouponPlanLinkV2")&&n.d(t,{getUpgradeWithCouponPlanLinkV2:function(){return b.getUpgradeWithCouponPlanLinkV2}}),n.o(b,"graphsStore")&&n.d(t,{graphsStore:function(){return b.graphsStore}}),n.o(b,"heliumExtensionInStore")&&n.d(t,{heliumExtensionInStore:function(){return b.heliumExtensionInStore}}),n.o(b,"isPositiveValue")&&n.d(t,{isPositiveValue:function(){return b.isPositiveValue}}),n.o(b,"isReadyStatus")&&n.d(t,{isReadyStatus:function(){return b.isReadyStatus}}),n.o(b,"isWalmartProductPageNew")&&n.d(t,{isWalmartProductPageNew:function(){return b.isWalmartProductPageNew}}),n.o(b,"isWalmartSearchPage")&&n.d(t,{isWalmartSearchPage:function(){return b.isWalmartSearchPage}}),n.o(b,"learnHowToUseLink")&&n.d(t,{learnHowToUseLink:function(){return b.learnHowToUseLink}}),n.o(b,"mainHeliumLink")&&n.d(t,{mainHeliumLink:function(){return b.mainHeliumLink}}),n.o(b,"membersLink")&&n.d(t,{membersLink:function(){return b.membersLink}}),n.o(b,"parserConfigLink")&&n.d(t,{parserConfigLink:function(){return b.parserConfigLink}}),n.o(b,"priceFromApi")&&n.d(t,{priceFromApi:function(){return b.priceFromApi}}),n.o(b,"queryClient")&&n.d(t,{queryClient:function(){return b.queryClient}}),n.o(b,"realTokenLink")&&n.d(t,{realTokenLink:function(){return b.realTokenLink}}),n.o(b,"renderElement")&&n.d(t,{renderElement:function(){return b.renderElement}}),n.o(b,"researchLink")&&n.d(t,{researchLink:function(){return b.researchLink}}),n.o(b,"reviewGraphDataLink")&&n.d(t,{reviewGraphDataLink:function(){return b.reviewGraphDataLink}}),n.o(b,"salesGraphDataLink")&&n.d(t,{salesGraphDataLink:function(){return b.salesGraphDataLink}}),n.o(b,"salesLink")&&n.d(t,{salesLink:function(){return b.salesLink}}),n.o(b,"searchGoogleGraphDataLink")&&n.d(t,{searchGoogleGraphDataLink:function(){return b.searchGoogleGraphDataLink}}),n.o(b,"signupLink")&&n.d(t,{signupLink:function(){return b.signupLink}}),n.o(b,"toggleExtension")&&n.d(t,{toggleExtension:function(){return b.toggleExtension}}),n.o(b,"tokenConnectionLink")&&n.d(t,{tokenConnectionLink:function(){return b.tokenConnectionLink}}),n.o(b,"transReactQueryStatus")&&n.d(t,{transReactQueryStatus:function(){return b.transReactQueryStatus}}),n.o(b,"transformIntoCNLink")&&n.d(t,{transformIntoCNLink:function(){return b.transformIntoCNLink}}),n.o(b,"upgradePlanLink")&&n.d(t,{upgradePlanLink:function(){return b.upgradePlanLink}}),n.o(b,"upgradeWithCouponPlanLinkV1")&&n.d(t,{upgradeWithCouponPlanLinkV1:function(){return b.upgradeWithCouponPlanLinkV1}}),n.o(b,"upsellDataLink")&&n.d(t,{upsellDataLink:function(){return b.upsellDataLink}}),n.o(b,"useEntitlements")&&n.d(t,{useEntitlements:function(){return b.useEntitlements}}),n.o(b,"useGetCacheStore")&&n.d(t,{useGetCacheStore:function(){return b.useGetCacheStore}}),n.o(b,"useGetStore")&&n.d(t,{useGetStore:function(){return b.useGetStore}}),n.o(b,"useModalState")&&n.d(t,{useModalState:function(){return b.useModalState}}),n.o(b,"useShowSpinner")&&n.d(t,{useShowSpinner:function(){return b.useShowSpinner}}),n.o(b,"useTranslation")&&n.d(t,{useTranslation:function(){return b.useTranslation}}),n.o(b,"waitForProduct")&&n.d(t,{waitForProduct:function(){return b.waitForProduct}}),n.o(b,"waitForSalesData")&&n.d(t,{waitForSalesData:function(){return b.waitForSalesData}}),n.o(b,"xrayInvocationLink")&&n.d(t,{xrayInvocationLink:function(){return b.xrayInvocationLink}});var C=n(87064),M=n(13217),L=n(95633);n(57677),n(74938)},88805:()=>{},99650:(e,t,n)=>{"use strict";n.d(t,{B3:()=>r});const r=(e,t,n)=>`${e}-${t}-${n}`},14833:(e,t,n)=>{"use strict";n.d(t,{e:()=>r});const r=e=>({id:e.results.id,accountId:e.results.accountId,userId:e.results.userId,hasMerchantIds:e.results.merchant_ids.length>0,hasTiktokToken:Boolean(e.results.hasTiktokToken)})},79802:()=>{},47801:()=>{},73743:(e,t,n)=>{"use strict";n.d(t,{w:()=>r});const r=e=>({accountId:e.accountId,logged:e.logged,upsellInfo:e.upsellInfo,onboardingStatus:e.onboardingStatus,activeExperiments:e.activeExperiments})},79939:()=>{},74633:()=>{},59624:()=>{},18054:()=>{},8394:(e,t,n)=>{"use strict"},56827:()=>{},11469:()=>{},87064:(e,t,n)=>{"use strict";n.d(t,{P2:()=>r,gz:()=>o});const r="https://www.amazon.com/",o=e=>`${e}/gp/customer-preferences/ajax/icp.html?icpContent=icp`},13217:(e,t,n)=>{"use strict";n.d(t,{D3:()=>i,GZ:()=>l,HF:()=>o,Ko:()=>u,R1:()=>a,XN:()=>s,XP:()=>r,r8:()=>c});const r="https://www.helium10.com/",o="https://research-tools.helium10.com",a="https://control-center.helium10.com",i="https://forms.gle/6VPbeYcGijeSRKeq6",l="https://survey.survicate.com/678bae536ee000df/?p=anonymous",s="https://chrome.google.com/webstore/detail/helium-10/njmehopjdpcckochcggncklnlmikcbnb",u="https://kb.helium10.com/hc/en-us/categories/20543704378139-Helium-10-s-Chrome-Extension",c=(e,t)=>{const n=new URL(`${o}/api/trends/get-google-trends`);return n.searchParams.append("period","year"),n.searchParams.append("keywords[]",t),null!=e&&n.searchParams.append("accountId",e),n.toString()}},95633:(e,t,n)=>{"use strict";n.d(t,{$w:()=>S,Bh:()=>v,Bx:()=>L,Cn:()=>s,DE:()=>w,I6:()=>M,JY:()=>p,Mn:()=>d,OG:()=>u,Ps:()=>b,Z0:()=>h,Z1:()=>I,fn:()=>E,hc:()=>P,iH:()=>y,j1:()=>k,j_:()=>c,mS:()=>g,nJ:()=>f,sD:()=>C,sI:()=>T,t9:()=>j});var r=Object.defineProperty,o=Object.getOwnPropertySymbols,a=Object.prototype.hasOwnProperty,i=Object.prototype.propertyIsEnumerable,l=(e,t,n)=>t in e?r(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n;const s="https://members.helium10.com",u=e=>e.replace(s,"https://members.h10.com.cn"),c=`${s}/user/signup?type=chrome-extension`,d=e=>{const t=new URL(`${s}/api/v1/customers/subscription/data`);return Object.entries(((e,t)=>{for(var n in t||(t={}))a.call(t,n)&&l(e,n,t[n]);if(o)for(var n of o(t))i.call(t,n)&&l(e,n,t[n]);return e})({},e)).map((([e,n])=>n?t.searchParams.append(e,n.toString()):void 0)),t.toString()},p=`${s}/api/v1/chrome-extension/get-country-by-seller`,g=`${s}/new-account/tokens-manager/add`,h=()=>{var e;const t=new URL(`${s}/extension/check-login`);return(null==(e=null==chrome?void 0:chrome.runtime)?void 0:e.id)&&t.searchParams.append("appId",chrome.runtime.id),t.toString()},m=`${s}/api/v1`,f=e=>{const t=new URL(`${m}/tiktok/review/charge-use`);return t.searchParams.set("accountId",String(e)),t.toString()},v=e=>{const t=new URL(`${m}/site/token`);return t.searchParams.append("accountId",e),t.toString()},y=`${m}/customers/segment-track`,k=`${m}/site/get-parser-config`,x="SEETHRU25",w=e=>{switch(e){case"ce_Xray":return"SEETHRU20_XRAY";case"xrayInfluencer":case"BSRWidgetInfluencer":case"OrderPageWidgetInfluencer":case"SERPWidgetInfluencer":case"BSRToolbarInfluencer":case"OrderPageToolbarInfluencer":case"SERPToolbarInfluencer":return"SEETHRU20_XRAYINFLUENCERS";case"ce_XrayKeywords":return"SEETHRU20_XRAYKEYWORDS";case"ce_SearchExpander":return"SEETHRU20_SEARCHEXPANDER";case"ce_BSRWidget":return"SEETHRU20_BSRWIDGET";case"ce_ProfitabilityCalculator":return"SEETHRU20_PROFITCALC";case"ce_DemandAnalyzer":return"SEETHRU20_DA";case"productIdea":return"SEETHRU20_PDP_PRODUCTIDEA";default:return x}},b=`${s}/subscribe`,C=`${b}?coupon=${x}`,M=e=>`${b}?coupon=${w(e)}`,L=(e,t)=>`${b}?offer=${"platinum"===e?"start_your_business":"scale_your_business"}&action=checkout&coupon=${w(t)}`,S=`${s}/black-box/invocations`,j=`${s}/black-box/sales`,I=`${m}/product/sales-chart`,T=`${m}/product/bsr-chart`,P=`${m}/product/review-chart`,E=`${m}/chrome-extension/get-top-3-asins-by-phrase`},4866:(e,t,n)=>{"use strict";n.d(t,{gy:()=>a,u_:()=>i,zM:()=>o});const r="https://h10api.pacvue.com",o=`${r}/customer-api/api/v1/customers/me`,a=`${r}/customer-api/api/v1/customers/me/chrome-extension`,i=`${r}/customer-api/api/v1/customers/set-onboarding-completed`},57677:(e,t,n)=>{"use strict";n.d(t,{b:()=>se});var r=n(27552),o=n(4866),a=n(62246),i=n(31810),l=Object.defineProperty,s=Object.defineProperties,u=Object.getOwnPropertyDescriptors,c=Object.getOwnPropertySymbols,d=Object.prototype.hasOwnProperty,p=Object.prototype.propertyIsEnumerable,g=(e,t,n)=>t in e?l(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n,h=(e,t)=>{for(var n in t||(t={}))d.call(t,n)&&g(e,n,t[n]);if(c)for(var n of c(t))p.call(t,n)&&g(e,n,t[n]);return e},m=(e,t)=>s(e,u(t)),f=(e,t,n)=>new Promise(((r,o)=>{var a=e=>{try{l(n.next(e))}catch(e){o(e)}},i=e=>{try{l(n.throw(e))}catch(e){o(e)}},l=e=>e.done?r(e.value):Promise.resolve(e.value).then(a,i);l((n=n.apply(e,t)).next())}));const v=e=>f(void 0,null,(function*(){var t,n,o;const a=null==(o=null==(n=null==(t=yield(0,r.onGetStoreOnce)("local","PACVUE_TOKENS"))?void 0:t.PACVUE_TOKENS)?void 0:n[e])?void 0:o.pacvueToken;return a?{"x-pacvue-token":`Bearer ${a}`}:{}})),y=(e,t,...n)=>f(void 0,[e,t,...n],(function*(e,t,n={}){const o=yield v(t),l=yield fetch(e,m(h({},n),{headers:h(h({},n.headers),o)}));if(401!==l.status)return l;yield(e=>f(void 0,null,(function*(){var t;a.d&&console.log("TOKEN - 401 detected, refreshing pacvue token for",e);const n=yield(0,i.B)(e);(null==(t=null==n?void 0:n.data)?void 0:t.pacvueToken)?(yield(0,r.updateStore)("local","PACVUE_TOKENS",(t=>m(h({},t),{[e]:{pacvueToken:n.data.pacvueToken,pacvueRefreshToken:n.data.pacvueRefreshToken,pacvueTokenExpiresIn:n.data.pacvueTokenExpiresIn,pacvueTokenCreatedAt:n.data.pacvueTokenCreatedAt}}))),a.d&&console.log("TOKEN - pacvue token refreshed successfully for",e)):a.d&&console.log("TOKEN - refresh returned no token, skipping cache update")})))(t);const s=yield v(t);return fetch(e,m(h({},n),{headers:h(h({},n.headers),s)}))}));var k=n(6892);const x=e=>{return t=void 0,n=null,a=function*(){const t=yield(0,r.updateLinkWithAccountId)(o.zM);return y(t,`${e}`).then((e=>(0,k.L)(e)))},new Promise(((e,r)=>{var o=e=>{try{l(a.next(e))}catch(e){r(e)}},i=e=>{try{l(a.throw(e))}catch(e){r(e)}},l=t=>t.done?e(t.value):Promise.resolve(t.value).then(o,i);l((a=a.apply(t,n)).next())}));var t,n,a};const w=(e,t)=>{return n=void 0,o=null,i=function*(){try{const n=t?void 0:yield b(e),o=n||(yield C(e));if(o)return a.d&&console.log("INFO - SET DATA",o),n||(yield(0,r.setStaticCachedData)("cache-sync","token",o,1)),o;a.d&&console.log("INFO - ERROR DATA: INVALID DATA")}catch(e){a.d&&console.log("INFO - ERROR DATA",e)}return null},new Promise(((e,t)=>{var r=e=>{try{l(i.next(e))}catch(e){t(e)}},a=e=>{try{l(i.throw(e))}catch(e){t(e)}},l=t=>t.done?e(t.value):Promise.resolve(t.value).then(r,a);l((i=i.apply(n,o)).next())}));var n,o,i},b=e=>(0,r.onGetCacheDataOnce)("cache-sync","token").then((({token:t})=>(a.d&&console.log("INFO - CACHE DATA",t),(null==t?void 0:t.accountId)===e?t:null))),C=e=>x(e).then((t=>(a.d&&console.log("INFO - FETCH DATA",t),(null==t?void 0:t.results.accountId)===e?(0,r.reduceAccountInfo)(t):null)));var M=n(9720),L=(e,t,n)=>new Promise(((r,o)=>{var a=e=>{try{l(n.next(e))}catch(e){o(e)}},i=e=>{try{l(n.throw(e))}catch(e){o(e)}},l=e=>e.done?r(e.value):Promise.resolve(e.value).then(a,i);l((n=n.apply(e,t)).next())}));const S=(e,t)=>L(void 0,null,(function*(){const n=t?(0,r.transformIntoCNLink)(e):e;return fetch(n,{}).then((e=>(0,k.L)(e,{accountId:-1,logged:!1})))}));var j=Object.defineProperty,I=Object.defineProperties,T=Object.getOwnPropertyDescriptors,P=Object.getOwnPropertySymbols,E=Object.prototype.hasOwnProperty,D=Object.prototype.propertyIsEnumerable,O=(e,t,n)=>t in e?j(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n,A=(e,t)=>{for(var n in t||(t={}))E.call(t,n)&&O(e,n,t[n]);if(P)for(var n of P(t))D.call(t,n)&&O(e,n,t[n]);return e};const R=(e,t)=>{return n=void 0,o=null,i=function*(){try{const n=t?null:yield N(e),o=n||(yield B(e));if(null==o?void 0:o.logged){const t=yield(0,r.onGetStoreOnce)("sync",["onboardingFinished"]).then((({onboardingFinished:e})=>e||{})),i=!(null==t?void 0:t[o.accountId])&&o.onboardingStatus;yield(0,r.onGetStoreOnce)("local",["accountId","CURRENT_ACCOUNT_ID","canApplyCoupon"]).then((({accountId:e,CURRENT_ACCOUNT_ID:t,canApplyCoupon:n})=>{var a,i;const l=!(o.accountId===e&&o.accountId===t),s=((null==(a=o.upsellInfo)?void 0:a.canApplyCoupon)||!1)!==n;if(l||s)return(0,r.setStore)("local",{accountId:o.accountId,CURRENT_ACCOUNT_ID:o.accountId,canApplyCoupon:(null==(i=o.upsellInfo)?void 0:i.canApplyCoupon)||!1})})),yield(0,r.onGetStoreOnce)("sync",["accountId","activeExperiments"]).then((({accountId:e,activeExperiments:n})=>{const a=o.accountId!==e,l=JSON.stringify(o.activeExperiments)!==JSON.stringify(n);if(a||l||i)return(0,r.setStore)("sync",{accountId:o.accountId,activeExperiments:o.activeExperiments||[],onboardingFinished:i?(s=A({},t),u={[o.accountId]:!0},I(s,T(u))):t});var s,u}));const l=yield(0,r.onGetCacheDataOnce)("cache-sync","login").then((({login:t})=>!!e&&(null==t?void 0:t.accountId)!==e));return a.d&&console.log("LOGIN - SET DATA",o,l?"CHANGED ACCOUNT!":""),l&&(yield(0,r.clearCachedData)("cache-sync",["login","token","entitlements","parser"])),!l&&n||(yield(0,r.setStaticCachedData)("cache-sync","login",o)),o}a.d&&console.log("LOGIN - ERROR DATA: NOT LOGGED IN!")}catch(e){a.d&&console.log("LOGIN - ERROR DATA",e)}return yield(0,r.setStore)("local",{accountId:-1,CURRENT_ACCOUNT_ID:0,canApplyCoupon:!1}),yield(0,r.setStore)("sync",{accountId:-1,isUserLogged:!1,activeExperiments:[]}),(0,r.clearCachedData)("cache-sync",["login","token","entitlements","parser"]).then(),(0,r.setStore)("local",{[r.TOKEN_KEY]:"{}"}).then(),null},new Promise(((e,t)=>{var r=e=>{try{l(i.next(e))}catch(e){t(e)}},a=e=>{try{l(i.throw(e))}catch(e){t(e)}},l=t=>t.done?e(t.value):Promise.resolve(t.value).then(r,a);l((i=i.apply(n,o)).next())}));var n,o,i},N=e=>(0,r.onGetCacheDataOnce)("cache-sync","login").then((({login:t})=>(a.d&&console.log("LOGIN - CACHE DATA",t),e&&(null==t?void 0:t.accountId)!==e?null:(null==t?void 0:t.logged)?t:null))),B=e=>L(void 0,null,(function*(){yield(0,M.H)("extensionConfig");const e=yield(0,r.updateLinkWithAccountId)((0,r.getCheckLoginLink)());yield(0,r.setStore)("sync",{isChinese:!1});const t=yield S(e,!1);if(!t.logged){const t=yield S(e,!0);if(!t.logged)throw new Error("No account found");return yield(0,r.setStore)("sync",{isChinese:!0}),t.amazonTokens=[],t}return t.amazonTokens=[],t})).then((t=>(a.d&&console.log("LOGIN - FETCH DATA",t),e&&(null==t?void 0:t.accountId)!==e?null:t?(0,r.reduceCheckLogin)(t):null)));const $=()=>{return e=void 0,t=null,n=function*(){const e=yield(0,r.updateLinkWithAccountId)(r.parserConfigLink);return fetch(e,{}).then((e=>(0,k.L)(e))).then((e=>{var t;return(null==(t=null==e?void 0:e.results)?void 0:t.config)||{}}))},new Promise(((r,o)=>{var a=e=>{try{l(n.next(e))}catch(e){o(e)}},i=e=>{try{l(n.throw(e))}catch(e){o(e)}},l=e=>e.done?r(e.value):Promise.resolve(e.value).then(a,i);l((n=n.apply(e,t)).next())}));var e,t,n};const G=()=>{return e=void 0,t=null,n=function*(){try{const e=yield F(),t=e||(yield z());if(t)return a.d&&console.log("PARSER - SET DATA",t),e||(yield(0,r.setStaticCachedData)("cache-sync","parser",t)),t;a.d&&console.log("PARSER - ERROR DATA: INVALID DATA")}catch(e){a.d&&console.log("PARSER - ERROR DATA",e)}return null},new Promise(((r,o)=>{var a=e=>{try{l(n.next(e))}catch(e){o(e)}},i=e=>{try{l(n.throw(e))}catch(e){o(e)}},l=e=>e.done?r(e.value):Promise.resolve(e.value).then(a,i);l((n=n.apply(e,t)).next())}));var e,t,n},F=()=>(0,r.onGetCacheDataOnce)("cache-sync","parser").then((({parser:e})=>(a.d&&console.log("PARSER - CACHE DATA",e),e||null))),z=()=>$().then((e=>(a.d&&console.log("PARSER - FETCH DATA",e),e)));var U=n(50576),_=n(69554);const W=e=>{return t=void 0,n=null,a=function*(){const t=yield(0,r.updateLinkWithAccountId)(o.gy);return void 0===e?fetch(t).then((e=>(0,k.L)(e))):y(t,`${e}`).then((e=>(0,k.L)(e)))},new Promise(((e,r)=>{var o=e=>{try{l(a.next(e))}catch(e){r(e)}},i=e=>{try{l(a.throw(e))}catch(e){r(e)}},l=t=>t.done?e(t.value):Promise.resolve(t.value).then(o,i);l((a=a.apply(t,n)).next())}));var t,n,a};var V=(e,t,n)=>new Promise(((r,o)=>{var a=e=>{try{l(n.next(e))}catch(e){o(e)}},i=e=>{try{l(n.throw(e))}catch(e){o(e)}},l=e=>e.done?r(e.value):Promise.resolve(e.value).then(a,i);l((n=n.apply(e,t)).next())}));const H=()=>(0,r.onGetCacheDataOnce)("cache-sync","entitlements").then((({entitlements:e})=>(a.d&&console.log("ENTITLEMENTS - CACHE DATA",e),e||null))),Z=e=>W(e).then((e=>(a.d&&console.log("ENTITLEMENTS - FETCH DATA",e),e||null)));var K=Object.defineProperty,Q=Object.defineProperties,Y=Object.getOwnPropertyDescriptors,X=Object.getOwnPropertySymbols,q=Object.prototype.hasOwnProperty,J=Object.prototype.propertyIsEnumerable,ee=(e,t,n)=>t in e?K(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n,te=(e,t)=>{for(var n in t||(t={}))q.call(t,n)&&ee(e,n,t[n]);if(X)for(var n of X(t))J.call(t,n)&&ee(e,n,t[n]);return e},ne=(e,t,n)=>new Promise(((r,o)=>{var a=e=>{try{l(n.next(e))}catch(e){o(e)}},i=e=>{try{l(n.throw(e))}catch(e){o(e)}},l=e=>e.done?r(e.value):Promise.resolve(e.value).then(a,i);l((n=n.apply(e,t)).next())}));const re=e=>ne(void 0,null,(function*(){const[{[r.TOKEN_KEY]:t},{PACVUE_TOKENS:n}]=yield Promise.all([(0,r.onGetStoreOnce)("local",r.TOKEN_KEY),(0,r.onGetStoreOnce)("local","PACVUE_TOKENS")]),o=ae(t),i=(null==o?void 0:o[e])||null;if(a.d&&console.log("TOKEN - CACHE DATA",o,e,i),!i)return null;const l=null==n?void 0:n[e];return!(null==l?void 0:l.pacvueToken)||(s=l.pacvueTokenCreatedAt,u=l.pacvueTokenExpiresIn,!Number.isFinite(s)||!Number.isFinite(u)||Date.now()>=s+1e3*u)?(a.d&&console.log((null==l?void 0:l.pacvueToken)?"TOKEN - PACVUE TOKEN EXPIRED, forcing re-fetch":"TOKEN - PACVUE TOKEN MISSING, forcing re-fetch"),null):i;var s,u})),oe=e=>ne(void 0,null,(function*(){var t,n;const o=yield(0,i.B)(e);a.d&&console.log("TOKEN - FETCH DATA",o);const l=null==(t=null==o?void 0:o.data)?void 0:t.pacvueToken;return l&&(yield(0,r.updateStore)("local","PACVUE_TOKENS",(t=>{return n=te({},t),r={[e]:{pacvueToken:l,pacvueRefreshToken:o.data.pacvueRefreshToken,pacvueTokenExpiresIn:o.data.pacvueTokenExpiresIn,pacvueTokenCreatedAt:o.data.pacvueTokenCreatedAt}},Q(n,Y(r));var n,r}))),(null==(n=null==o?void 0:o.data)?void 0:n.token)||null})),ae=e=>{try{return JSON.parse(e||"{}")||{}}catch(e){return console.log("~ parseToken ~ error:",e),{}}};var ie=(e,t,n)=>new Promise(((r,o)=>{var a=e=>{try{l(n.next(e))}catch(e){o(e)}},i=e=>{try{l(n.throw(e))}catch(e){o(e)}},l=e=>e.done?r(e.value):Promise.resolve(e.value).then(a,i);l((n=n.apply(e,t)).next())}));const le={value:!1},se=e=>ie(void 0,null,(function*(){if(a.d&&console.log("GETTING FULL LOGIN",le,e),le.value&&!e.forced)return yield new Promise((e=>setTimeout(e,1e3))),[];let t=!1;for(;le.value;)a.d&&console.log("SLEEPING DUPLICATE LOGIN PROCESS!"),t=!0,yield new Promise((e=>setTimeout(e,1e3)));return le.value=!0,R(e.targetId,!t&&(null==e?void 0:e.ignoreCache)).then((t=>ie(void 0,null,(function*(){if(null==t?void 0:t.accountId){const i=yield(o=t.accountId,ne(void 0,null,(function*(){const e=`${o}`,t=yield re(e),n=t||(yield oe(e));return n&&!t&&(yield(0,r.updateStore)("local",r.TOKEN_KEY,(t=>JSON.stringify(te(te({},ae(t)),e?{[e]:n}:{}))))),{data:a.d?yield(0,r.onGetStoreOnce)("local",r.TOKEN_KEY).then((({[r.TOKEN_KEY]:e})=>`${e}`)):"--DISABLED LOG--",token:n,accountId:e}}))),[l,s,u]=yield Promise.all([w(t.accountId,null==e?void 0:e.updateInfoData),(n=t.accountId,V(void 0,null,(function*(){try{const e=yield H(),t=e||(yield Z(n)),o=!!t;if(yield(0,r.onGetStoreOnce)("sync",["isUserLogged","extensionEnabled","settings"]).then((({isUserLogged:e,extensionEnabled:t,settings:n})=>{const a={};e!==o&&(a.isUserLogged=o),o&&null===t&&(a.extensionEnabled=!0),o&&null===n&&(a.settings=_.D),Object.keys(a).length>0&&(0,r.setStore)("sync",a)})),t)return a.d&&console.log("ENTITLEMENTS - SET DATA",t),e?Z(n).then((e=>V(void 0,null,(function*(){if(!e)return;const n=(0,k.R)(t),o=(0,k.R)(e);!(0,U.Z)(n,o)&&(yield(0,r.clearCachedData)("cache-sync",["entitlements"]),chrome.tabs.reload())})))):yield(0,r.setStaticCachedData)("cache-sync","entitlements",t),t;a.d&&console.log("ENTITLEMENTS - ERROR DATA: INVALID DATA")}catch(e){a.d&&console.log("ENTITLEMENTS - ERROR DATA",e)}return null}))),G()]);return le.value=!1,{result:t,tokenData:i,infoData:l,planData:s,parserData:u}}var n,o;return le.value=!1,{result:t,tokenData:null,infoData:null,planData:null,parserData:null}})))).then((t=>{var n,r,o;return a.d&&console.log(`LOGIN RESULT - ${null==(n=e.log)?void 0:n.toUpperCase()}`,null==(r=null==t?void 0:t.result)?void 0:r.accountId,null==(o=null==t?void 0:t.planData)?void 0:o.data.plan.key,{params:e,results:t}),t})).catch((()=>{le.value=!1}))}))},62246:(e,t,n)=>{"use strict";n.d(t,{d:()=>r});const r=!1},69554:(e,t,n)=>{"use strict";n.d(t,{D:()=>r});const r={extendedInformation:!0,listingHealthScore:!0,bsrGraph:!0,pageWidget:!0,searchExpander:!0,searchPageToolbar:!0,findSimilar:!0,keywordTracker:!0,factor:"two",score:{revenue:5e3,reviews:75},language:"en"}},1013:(e,t,n)=>{"use strict";var r=n(27552),o=n(27044),a=n(82274);const i={};var l=n(29444),s=n(65378),u=Object.defineProperty,c=Object.getOwnPropertySymbols,d=Object.prototype.hasOwnProperty,p=Object.prototype.propertyIsEnumerable,g=(e,t,n)=>t in e?u(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n,h=(e,t)=>{for(var n in t||(t={}))d.call(t,n)&&g(e,n,t[n]);if(c)for(var n of c(t))p.call(t,n)&&g(e,n,t[n]);return e};let m=!1;const f={"h10-asin-grabber":"open-asin-grabber","h10-profitability-calculator":"open-calculator","h10-profitability-calculator-walmart":"open-calculator-walmart","h10-inventory":"open-inventory","h10-review":"open-review-insights","h10-xray":"open-black-box","h10-xray-walmart":"open-xray-walmart","h10-xray-keywords":"open-xray-keywords","h10-ai-review-insights":"open-ai-review-insights","h10-category-review-returns":"open-category-review-returns","h10-tiktok-review-insights":"open-tiktok-review-insights"},v=(e,t)=>{if(e.type){const n=(0,o.getModalState)(e.type);if(n){let r=e.params;return e.type===o.tabMessageTypes.OPEN_CATEGORY_REVIEW_RETURNS&&(r=h(h({},s.I.getModalParams()),e.params)),n.open(r,f[e.type]),null==t||t(!0),!0}switch(e.type){case o.tabMessageTypes.AMAZON_PRODUCT:null==t||t(y());break;case o.tabMessageTypes.WALMART_PRODUCT:null==t||t(k());break;case o.tabMessageTypes.GET_ASIN_ORIGINS:null==t||t((0,a.checkAsinsOrigins)());break;case o.tabMessageTypes.TIKTOK_PRODUCT:null==t||t(x());break;case o.tabMessageTypes.IS_FILES_INJECTED:null==t||t({status:!0});break;case o.tabMessageTypes.OPEN_UPSELL_MODAL:(0,l.p)(e.params).openToolSpecific(),null==t||t(!0);break;case o.tabMessageTypes.GET_CATEGORY_REVIEW_RETURNS_AVAILABLE:null==t||t({available:s.I.getAvailable()})}}return null==t||t(!1),!0},y=()=>{const e=(0,o.getCurrentMarketplace)();if((0,a.extractAsinFromUrl)())return{status:"product",marketplace:e};if(document.getElementById("#s-results-list-atf")||document.getElementById("#zg_centerListWrapper")||document.querySelector(".zg-item-immersion .a-list-item")||document.querySelector(".s-result-list .s-result-item ")||document.querySelector(".p13n-gridRow #gridItemRoot"))return{status:"listing",marketplace:e};return document.querySelector(".your-orders-content-container")?{status:"order",marketplace:e}:{status:null,marketplace:e}},k=()=>{const e=(0,o.getCurrentMarketplace)();return document.location.href.match(/https?:\/{2}(?:w{3}\.)?(walmart\.com\/ip\/)/)?{status:"product",marketplace:e}:/\/browse\//i.test(document.location.href)||/\/search?/i.test(document.location.href)?{status:"listing",marketplace:e}:{status:null,marketplace:e}},x=()=>{const e=(0,o.getCurrentMarketplace)();return(0,a.isTiktokProductPage)()?{status:"product",marketplace:e}:{status:null,marketplace:e}},w=!1;var b=n(81381),C=Object.defineProperty,M=Object.getOwnPropertySymbols,L=Object.prototype.hasOwnProperty,S=Object.prototype.propertyIsEnumerable,j=(e,t,n)=>t in e?C(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n;let I=!1,T=!1;const P=()=>{return e=void 0,t=null,n=function*(){I=!0,T=!1;try{yield(0,r.clearCachedData)("cache-sync",["login","token","entitlements","parser"]),yield(0,r.setStore)("local",{[r.TOKEN_KEY]:"{}"});const{accountId:e}=yield(0,r.onGetStoreOnce)("sync","accountId");e&&(yield(0,r.updateStore)("local","PACVUE_TOKENS",(t=>{if(!t)return{};const n=((e,t)=>{for(var n in t||(t={}))L.call(t,n)&&j(e,n,t[n]);if(M)for(var n of M(t))S.call(t,n)&&j(e,n,t[n]);return e})({},t);return delete n[`${e}`],n}))),yield(0,r.sendBackgroundLoginMessage)({log:"401 REFRESH",ignoreCache:!0})}finally{I=!1,T&&(T=!1,P().then())}},new Promise(((r,o)=>{var a=e=>{try{l(n.next(e))}catch(e){o(e)}},i=e=>{try{l(n.throw(e))}catch(e){o(e)}},l=e=>e.done?r(e.value):Promise.resolve(e.value).then(a,i);l((n=n.apply(e,t)).next())}));var e,t,n};var E=n(85893),D=n(67294),O=n(2825),A=n(17625),R=n(61193),N=n.n(R),B=n(12788),$=n(19302),G=n(76248),F=n(98641),z=n(1203),U=n(52332),_=n(70533),W=n(50161),V=n(67749),H=n(4866);const Z=e=>{return t=void 0,n=null,r=function*(){const t=yield(0,o.updateLinkWithAccountId)(H.u_);return(0,V.H)({url:t,type:o.backgroundMessageTypes.BACKGROUND_REQUEST,method:"POST",params:{completedStatus:e},useSessionAccountId:!0})},new Promise(((e,o)=>{var a=e=>{try{l(r.next(e))}catch(e){o(e)}},i=e=>{try{l(r.throw(e))}catch(e){o(e)}},l=t=>t.done?e(t.value):Promise.resolve(t.value).then(a,i);l((r=r.apply(t,n)).next())}));var t,n,r};var K=Object.defineProperty,Q=Object.defineProperties,Y=Object.getOwnPropertyDescriptors,X=Object.getOwnPropertySymbols,q=Object.prototype.hasOwnProperty,J=Object.prototype.propertyIsEnumerable,ee=(e,t,n)=>t in e?K(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n,te=(e,t)=>{for(var n in t||(t={}))q.call(t,n)&&ee(e,n,t[n]);if(X)for(var n of X(t))J.call(t,n)&&ee(e,n,t[n]);return e};const ne=(0,W.F)(((e,t)=>te(te(te(te({},(0,F.a)(e,t,"isDrag",!1)),(0,F.a)(e,t,"showPopup",!1)),(0,F.a)(e,t,"marketplace",(0,z.yW)((0,a.isWalmartPage)()?"WM":"AM","store",(0,o.getMarketDataFromDomain)().store).domain)),(0,U.o)(e,t,"finishOnboarding",(e=>{return t=void 0,n=null,r=function*(){return yield Z(e)},new Promise(((e,o)=>{var a=e=>{try{l(r.next(e))}catch(e){o(e)}},i=e=>{try{l(r.throw(e))}catch(e){o(e)}},l=t=>t.done?e(t.value):Promise.resolve(t.value).then(a,i);l((r=r.apply(t,n)).next())}));var t,n,r}),{initialStatus:"init",fulfilledReaction:()=>{(0,o.onGetStoreOnce)("sync",["accountId","onboardingFinished"]).then((({accountId:e,onboardingFinished:t})=>{var n,r;e&&(0,o.setStore)("sync",{onboardingFinished:(n=te({},t),r={[e]:!0},Q(n,Y(r)))}).then()})),(0,o.onGetStoreOnce)("local","onboardingStep").then((({onboardingStep:e})=>{3===e&&(0,o.setStore)("local",{onboardingStep:1}).then()}))},rejectedReaction:e=>{const t=(0,o.getTranslation)();_.c.error((null==t?void 0:t.t("errorCodes.unknown"))||"Unknown server error")}}))));let re;const oe=e=>Math.max(Math.min(e||0,90),-1*(window.innerHeight-140)),ae=(e,t)=>{ne.getState().isDrag.set(!0)},ie=(e,{y:t})=>{(0,o.setStore)("sync",{widgetPosition:oe(t)}).then((()=>{ne.getState().isDrag.set(!1)}))},le=()=>{ne.getState().showPopup.set(!0),re&&clearTimeout(re)},se=()=>{re=setTimeout(ue,5e3)},ue=()=>{ne.getState().showPopup.set(!1)};var ce,de;function pe(){return pe=Object.assign?Object.assign.bind():function(e){for(var t=1;t<arguments.length;t++){var n=arguments[t];for(var r in n)({}).hasOwnProperty.call(n,r)&&(e[r]=n[r])}return e},pe.apply(null,arguments)}const ge=({marketplace:e})=>e.value,he=({isDrag:e})=>e.value,me=({showPopup:e})=>e.value,fe="extension-page-widget-position",ve="modalDragHandle",ye=()=>{const{isInit:e,widgetPosition:t}=(0,o.useGetStore)("sync","widgetPosition"),n=ne(he,G.X),r=(0,$.Z)((()=>{(0,o.updateStore)("sync","widgetPosition",oe).then()}),500);(0,D.useEffect)((()=>(window.addEventListener("resize",r),()=>window.removeEventListener("resize",r))),[r]),(0,D.useLayoutEffect)((()=>{chrome.storage.sync.get(fe,(e=>{e[fe]&&((0,o.setStore)("sync",{widgetPosition:oe(e[fe].y)}).then(),chrome.storage.sync.remove(fe).then())}))}),[]);const a=(0,D.useMemo)((()=>({x:0,y:"object"==typeof t?(null==t?void 0:t.y)||0:t||0})),[t]);return e?(0,E.jsx)(E.Fragment,{}):(0,E.jsx)(N(),{axis:"y",scale:1,defaultPosition:a,position:n?void 0:a,handle:`.${ve}`,enableUserSelectHack:!0,onStart:ae,onStop:ie,children:(0,E.jsxs)(we,{id:"h10-page-widget",$isDrag:n,onMouseEnter:le,onMouseLeave:se,children:[(0,E.jsx)(be,{children:(0,E.jsx)(Ce,{})}),(0,E.jsx)(xe,{className:ve,children:(0,E.jsx)(ke,{icon:O._3F})})]})})},ke=(0,B.default)(A.G)`
  * {
    color: #fff;
  }
`,xe=B.default.div`
  width: 0;
  background: ${({theme:e})=>e.systemColors.blue900};
  cursor: all-scroll;
  transition: all 0.2s;
  display: flex;
  align-self: stretch;
  align-items: center;
  overflow: hidden;
  box-sizing: border-box;
`,we=B.default.div`
  display: flex;
  position: fixed;
  bottom: ${90}px;
  right: 0;
  z-index: 999;
  color: #fff;
  background: ${({theme:e})=>e.systemColors.blue600};
  box-shadow:
    0 2px 3px rgba(103, 132, 162, 0.08),
    0 1px 8px rgba(103, 132, 162, 0.16);
  border-radius: ${({theme:e})=>`${e.spacing.sp6} 0 0 ${e.spacing.sp6}`};

  ${({$isDrag:e})=>e&&B.css`
      ${xe} {
        width: 23px;
        padding: ${({theme:e})=>`0 ${e.spacing.sp8}`};
      }
    `}
  &:hover {
    ${xe} {
      width: 23px;
      padding: ${({theme:e})=>`0 ${e.spacing.sp8}`};
    }
  }
`,be=B.default.div`
  font-size: 14px;
  font-weight: bold;
  display: flex;
  align-items: center;
`,Ce=(0,B.default)((e=>D.createElement("svg",pe({xmlns:"http://www.w3.org/2000/svg",fill:"none",viewBox:"0 0 89 86"},e),ce||(ce=D.createElement("path",{d:"M67.41 58.01H88.6v27.81H67.41V58.01ZM34 58.01h21.19v27.81H34V58.01ZM.6 58.01h21.19v27.81H.6V58.01Z","data-area":"icon-bottom"})),de||(de=D.createElement("path",{d:"M67.41 12.17v34.48H88.6V.6L67.41 12.17ZM34 30.41v16.24h21.2V18.83L34 30.41ZM.6 18.83v27.82h21.19V30.41L.6 18.83Z","data-area":"icon-top"})))))`
  width: 24px;
  height: auto;
  display: block;
  margin: ${({theme:e})=>`${e.spacing.sp12} ${e.spacing.sp16}`};

  & > path[data-area="icon-top"] {
    fill: #ffffff;
  }

  & > path[data-area="icon-bottom"] {
    fill: #ffffff;
  }
`;var Me=n(12003),Le=n(62135),Se=n(48372),je=n(94482),Ie=n(89107),Te=n(31541);B.default.div`
  padding: ${({theme:e})=>e.spacing.sp24};
  margin: auto 0;
  display: flex;
  flex-direction: column;
`;const Pe=B.default.div`
  width: 80px;
  height: 80px;
  border-radius: 50%;
  background-color: rgba(0, 129, 255, 0.15);
  margin: auto;
  display: flex;
  align-items: center;
  justify-content: center;
`,Ee=B.default.span`
  ${Ie.uh};
  margin: ${({theme:e})=>`${e.spacing.sp12} auto ${e.spacing.sp12}`};
  color: ${({theme:e})=>e.systemColors.blue600};
  text-align: center;
`,De=B.default.span`
  ${Ie.Qv};
  color: ${({theme:e})=>e.systemColors.black};
  margin-bottom: ${({theme:e})=>e.spacing.sp24};
  text-align: center;
`,Oe=(B.default.div`
  display: flex;
  justify-content: center;

  ${je.R}+${je.R} {
    margin-left: ${({theme:e})=>e.spacing.sp32};
  }
`,()=>{(0,o.sendBackgroundMessage)(o.backgroundMessageTypes.OPEN_PAGE,{url:Te.ie}).then()}),Ae=()=>{(0,o.sendBackgroundMessage)(o.backgroundMessageTypes.OPEN_PAGE,{url:o.signupLink}).then()},Re=()=>{const{t:e}=(0,o.useTranslation)();return(0,E.jsxs)(Ne,{children:[(0,E.jsx)(Pe,{children:(0,E.jsx)(o.LockIcon,{})}),(0,E.jsx)(Be,{dangerouslySetInnerHTML:{__html:e("auth.welcome")}}),(0,E.jsx)(De,{children:e("auth.logInText")}),(0,E.jsx)(je.R,{onClick:Oe,size:"200",children:e("auth.logIntoAccount")}),(0,E.jsx)($e,{children:e("auth.or")}),(0,E.jsx)(je.R,{onClick:Ae,size:"200",buttonType:"secondary",style:{fontSize:"16px"},children:e("auth.signUp")})]})},Ne=B.default.div`
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  margin: auto 0;

  ${je.R} {
    width: 100%;
  }
`,Be=(0,B.default)(Ee)`
  text-align: center;
`,$e=B.default.div`
  ${Ie.nO};
  margin-top: ${({theme:e})=>e.spacing.sp12};
  margin-bottom: ${({theme:e})=>e.spacing.sp12};
  width: 100%;
  overflow: hidden;
  text-align: center;

  &::before,
  &::after {
    content: "";
    display: inline-block;
    vertical-align: middle;
    box-sizing: border-box;
    width: 100%;
    height: 2px;
    background: ${({theme:e})=>e.systemColors.grey200};
    border: solid #fff;
    border-width: 0 10px;
  }

  &::before {
    margin-left: -100%;
  }

  &::after {
    margin-right: -100%;
  }
`;var Ge=n(83851),Fe=n(7654),ze=n(56541),Ue=Object.defineProperty,_e=Object.defineProperties,We=Object.getOwnPropertyDescriptors,Ve=Object.getOwnPropertySymbols,He=Object.prototype.hasOwnProperty,Ze=Object.prototype.propertyIsEnumerable,Ke=(e,t,n)=>t in e?Ue(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n;const Qe=()=>{const{mode:e}=(0,o.useGetStore)("sync","mode"),t=(0,ze.dO)(),n=(0,D.useCallback)((e=>{(0,o.setStore)("sync",{mode:e.value}),(0,o.updateStore)("sync","settings",(t=>{return n=((e,t)=>{for(var n in t||(t={}))He.call(t,n)&&Ke(e,n,t[n]);if(Ve)for(var n of Ve(t))Ze.call(t,n)&&Ke(e,n,t[n]);return e})({},t),r={searchExpander:"seller"===e.value,bsrGraph:"seller"===e.value},_e(n,We(r));var n,r})),(0,o.sendSegmentEventMessage)({name:o.segmentEventType.modeSwitcher,marketplace:(0,o.getCurrentMarketplace)(),properties:{mode:e.value,from:"extension"}}).then(),(0,o.sendBackgroundMessage)(o.backgroundMessageTypes.RELOAD_TABS).then()}),[]);return(0,E.jsx)(Ye,{size:"50",options:t,appendTo:"parent",value:e,onChange:n})},Ye=(0,B.default)(Fe.T)`
  width: 110px;
`,Xe={main:"common.extension",demandAnalyzer:"demandAnalyzer.title"},qe=({type:e})=>{const{t}=(0,o.useTranslation)();return(0,E.jsxs)(Je,{children:[(0,E.jsx)(et,{type:"small"}),(0,E.jsx)(tt,{children:t(Xe[e])})]})},Je=B.default.div`
  display: flex;
  align-items: center;
  gap: ${({theme:e})=>e.spacing.sp8};
`,et=(0,B.default)(Se.T9)`
  display: flex;
  width: 20px;
  line-height: normal;
`,tt=B.default.div`
  ${Ie.uh};
  color: ${({theme:e})=>e.systemColors.blue900};
`,nt=({isEnabled:e,isAmazon:t,isMainMenu:n=!0})=>{const{t:r}=(0,o.useTranslation)(),i=(0,Le.G)(),l=(0,D.useMemo)((()=>"seller"===i||!t),[t,i]),s=(0,D.useMemo)((()=>{var e;return(null==(e=chrome.runtime.getManifest().version_name)?void 0:e.toString())||""}),[]);return(0,E.jsxs)(rt,{children:[(0,E.jsxs)(ot,{children:[(0,E.jsx)(qe,{type:"main"}),(0,E.jsxs)(lt,{children:[t&&!(0,a.isAmazonImageOCRPage)()&&(0,E.jsx)(Qe,{}),(0,E.jsx)(ut,{size:"100",buttonType:"icon",icon:(0,E.jsx)(A.G,{icon:O.gr5}),onClick:()=>{(0,o.sendBackgroundMessage)(o.backgroundMessageTypes.OPEN_OPTIONS).then()}}),n?(0,E.jsx)(st,{checked:e,onChange:o.toggleExtension}):void 0]})]}),(0,E.jsxs)(at,{children:[(0,E.jsx)(ct,{children:l&&(0,E.jsx)(je.R,{buttonType:"text",onClick:()=>{(0,o.sendSegmentEventMessage)({name:o.segmentEventType.changeLogClicked,properties:{linkLocation:"main menu"}}).finally((()=>{(0,o.sendBackgroundMessage)(o.backgroundMessageTypes.OPEN_PAGE,{url:o.learnHowToUseLink}).then()}))},children:r("options.learnHowToUse")})}),(0,E.jsx)(dt,{children:(0,E.jsx)(it,{children:r("options.version",{version:s})})})]})]})},rt=B.default.div``,ot=B.default.div`
  flex: 0 0 auto;
  display: flex;
  justify-content: space-between;
  align-items: center;
`,at=B.default.div`
  display: flex;
  justify-content: space-between;
  gap: ${({theme:e})=>e.spacing.sp8};
  align-items: center;
`,it=B.default.div`
  font-size: 12px;
  color: ${({theme:e})=>e.systemColors.grey400};
`,lt=B.default.div`
  display: flex;
  gap: ${({theme:e})=>e.spacing.sp8};
`,st=(0,B.default)(Ge.$)``,ut=(0,B.default)(je.R)`
  > div {
    display: flex;
  }

  svg {
    font-size: 20px;
  }
`,ct=B.default.div`
  display: flex;
  flex-shrink: 0;
  margin: 4px 0;
`,dt=B.default.div`
  display: flex;
`;var pt=n(1220),gt=n(40974),ht=n(57383),mt=n(98377),ft=n(66609),vt=n(13025),yt=n(40957),kt=n(67758);const xt=()=>{const[e,t]=(0,D.useState)(!1);return{validateAsin:(e,n)=>{return r=void 0,o=null,a=function*(){try{t(!0);const r=(0,Te.Fd)(n,e);return yield(0,Te.fx)(r),t(!1),!0}catch(e){return t(!1),!1}},new Promise(((e,t)=>{var n=e=>{try{l(a.next(e))}catch(e){t(e)}},i=e=>{try{l(a.throw(e))}catch(e){t(e)}},l=t=>t.done?e(t.value):Promise.resolve(t.value).then(n,i);l((a=a.apply(r,o)).next())}));var r,o,a},isLoading:e}};const wt=["amazon.com","amazon.ca","amazon.com.mx","amazon.de","amazon.es","amazon.fr","amazon.it","amazon.co.uk","amazon.in","amazon.co.jp","amazon.com.au","amazon.nl","amazon.ae","amazon.com.br","amazon.sa","walmart.com"].map((e=>vt.bV.find((({value:t})=>t===e)))).filter((e=>null!==e)),bt=({fromOnboarding:e=!1})=>{const{t}=(0,o.useTranslation)(),[n,r]=(0,D.useState)(""),{validateAsin:a,isLoading:i}=xt(),l=(0,D.useMemo)((()=>!n.trim().length||i),[n,i]),s=(0,D.useCallback)((t=>{return r=void 0,i=null,l=function*(){t.preventDefault();const r=ge(ne.getState()),i=(yield a(n,r))?(0,Te.Fd)(r,n):(0,Te.Kr)(n,r);yield(0,o.sendBackgroundMessage)(r.includes("walmart")?o.backgroundMessageTypes.OPEN_PAGE_WALMART_XRAY:o.backgroundMessageTypes.OPEN_PAGE_XRAY,{url:i,location:e?"xrayOnboarding":"widgetMenuXray"})},new Promise(((e,t)=>{var n=e=>{try{a(l.next(e))}catch(e){t(e)}},o=e=>{try{a(l.throw(e))}catch(e){t(e)}},a=t=>t.done?e(t.value):Promise.resolve(t.value).then(n,o);a((l=l.apply(r,i)).next())}));var r,i,l}),[a,n,e]),u=(0,D.useCallback)((e=>e.stopPropagation()),[]);return(0,E.jsxs)(Mt,{onSubmit:s,onClick:u,children:[(0,E.jsx)(yt.t,{size:"200",disabled:i,placeholder:t("common.enterKeywordOrAsin"),value:n,onChange:e=>r(e.target.value),leftElement:(0,E.jsx)(Ct,{})}),(0,E.jsx)(Lt,{children:(0,E.jsx)(je.R,{size:"200",type:"submit",disabled:l,loading:i,children:t("common.runXray")})})]})},Ct=()=>{const e=(0,o.useModalPlaceContext)(),t=ne(ge,G.X),n=(0,D.useCallback)((({value:e})=>{ne.getState().marketplace.set(e)}),[]);return(0,E.jsx)(St,{size:"100",value:t,onChange:n,options:wt,appendTo:e,groupOptions:!0,onDropdownShow:()=>{},onDropdownHidden:()=>{}})},Mt=B.default.form`
  display: flex;
  flex-direction: column;
  margin-bottom: 0;

  ${yt.t.InputWrapper} {
    overflow: initial;
    min-width: 345px;
    width: 100%;
  }

  ${yt.t.Element} {
    padding-left: ${({theme:e})=>e.spacing.sp4};
  }
`,Lt=B.default.div`
  display: flex;
  width: 100%;
  margin-top: ${({theme:e})=>e.spacing.sp8};

  ${je.R} {
    width: 100%;
  }
`,St=(0,B.default)(kt.y)`
  > div {
    align-items: center;
  }
`;var jt=Object.defineProperty,It=Object.defineProperties,Tt=Object.getOwnPropertyDescriptors,Pt=Object.getOwnPropertySymbols,Et=Object.prototype.hasOwnProperty,Dt=Object.prototype.propertyIsEnumerable,Ot=(e,t,n)=>t in e?jt(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n,At=(e,t)=>{for(var n in t||(t={}))Et.call(t,n)&&Ot(e,n,t[n]);if(Pt)for(var n of Pt(t))Dt.call(t,n)&&Ot(e,n,t[n]);return e},Rt=(e,t)=>It(e,Tt(t));const Nt=({onAction:e,isSellerMode:t})=>{const n=(0,D.useMemo)((()=>(0,a.isAmazonProductPage)()),[]),r=(0,D.useMemo)((()=>(0,a.isAmazonSearchPage)()||(0,a.isAmazonBestSellersPage)()),[]),i=(0,D.useMemo)((()=>(0,a.isAmazonOrderPage)()),[]),l=(0,D.useMemo)((()=>({isProductPage:n,isSearchPage:r,isOrderPage:i})),[n,r,i]),s=(0,D.useMemo)((()=>({overrideAction:(t,n)=>{return r=void 0,o=null,a=function*(){return e(t,n)},new Promise(((e,t)=>{var n=e=>{try{l(a.next(e))}catch(e){t(e)}},i=e=>{try{l(a.throw(e))}catch(e){t(e)}},l=t=>t.done?e(t.value):Promise.resolve(t.value).then(n,i);l((a=a.apply(r,o)).next())}));var r,o,a},onPostClickAction:()=>ue()})),[e]),{features:u,availableMarkets:c}=(0,ft.Q)(),d=(0,ht.bN)(),p=(0,mt.S)();return(0,E.jsx)(Bt,{children:(0,E.jsx)($t,{children:n||r||i?(0,E.jsxs)(E.Fragment,{children:[(null==u?void 0:u.includes(ft.l.XRAY))&&(0,E.jsx)(o.MenuXrayButton,Rt(At(At({},l),s),{isWidget:!0})),t&&!i&&(null==u?void 0:u.includes(ft.l.XRAY_KEYWORDS))&&(0,E.jsx)(o.MenuXrayKeywordsButton,At(At({},l),s)),n&&(null==u?void 0:u.includes(ft.l.AI_REVIEW_INSIGHTS))&&(null==c?void 0:c.includes(d))&&(0,E.jsx)(pt.H,At(At({},l),s)),(n||p)&&(null==u?void 0:u.includes(ft.l.CATEGORY_REVIEW_RETURNS))&&(null==c?void 0:c.includes(d))&&(0,E.jsx)(gt.I,Rt(At(At({},l),s),{isProductPage:n||p})),t&&!i&&(null==u?void 0:u.includes(ft.l.ASIN_GRABBER))&&(0,E.jsx)(o.MenuAsinGrabberButton,At(At({},l),s)),n?(0,E.jsxs)(E.Fragment,{children:[t&&(null==u?void 0:u.includes(ft.l.CALCULATOR))&&(0,E.jsx)(o.MenuCalculatorButton,At(At({},l),s)),t&&(null==u?void 0:u.includes(ft.l.INVENTORY_LEVELS))&&(0,E.jsx)(o.MenuInventoryButton,At(At({},l),s))]}):void 0,(null==u?void 0:u.includes(ft.l.FEEDBACK))&&(0,E.jsx)(o.MenuFeedbackButton,At({},s))]}):(null==u?void 0:u.includes(ft.l.XRAY))&&(0,E.jsx)(bt,{})})})},Bt=(0,B.default)(Se.Rm)`
  display: flex;
  flex-grow: 1;
`,$t=B.default.div`
  display: flex;
  width: 100%;
  flex-direction: column;
  gap: ${({theme:e})=>e.spacing.sp8};
  margin-bottom: ${({theme:e})=>e.spacing.sp8};
`;var Gt=Object.defineProperty,Ft=Object.getOwnPropertySymbols,zt=Object.prototype.hasOwnProperty,Ut=Object.prototype.propertyIsEnumerable,_t=(e,t,n)=>t in e?Gt(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n,Wt=(e,t)=>{for(var n in t||(t={}))zt.call(t,n)&&_t(e,n,t[n]);if(Ft)for(var n of Ft(t))Ut.call(t,n)&&_t(e,n,t[n]);return e};const Vt=({onAction:e})=>{const t=(0,D.useMemo)((()=>(0,a.isTiktokProductPage)()),[]),{features:n}=(0,ft.Q)(),r=(0,D.useMemo)((()=>({overrideAction:(t,n)=>{return r=void 0,o=null,a=function*(){return e(t,n)},new Promise(((e,t)=>{var n=e=>{try{l(a.next(e))}catch(e){t(e)}},i=e=>{try{l(a.throw(e))}catch(e){t(e)}},l=t=>t.done?e(t.value):Promise.resolve(t.value).then(n,i);l((a=a.apply(r,o)).next())}));var r,o,a},onPostClickAction:()=>ue()})),[e]);return(0,E.jsx)(Ht,{children:(0,E.jsxs)(Zt,{children:[t&&(null==n?void 0:n.includes(ft.l.TIKTOK_REVIEW_INSIGHTS))&&(0,E.jsx)(o.MenuTiktokReviewInsightsButton,Wt({},r)),(null==n?void 0:n.includes(ft.l.TIKTOK_FEEDBACK))&&(0,E.jsx)(o.MenuFeedbackButton,Wt({},r))]})})},Ht=(0,B.default)(Se.Rm)`
  display: flex;
  flex-grow: 1;
`,Zt=B.default.div`
  display: flex;
  width: 100%;
  flex-direction: column;
  gap: ${({theme:e})=>e.spacing.sp8};
  margin-bottom: ${({theme:e})=>e.spacing.sp8};
`;var Kt=Object.defineProperty,Qt=Object.getOwnPropertySymbols,Yt=Object.prototype.hasOwnProperty,Xt=Object.prototype.propertyIsEnumerable,qt=(e,t,n)=>t in e?Kt(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n,Jt=(e,t)=>{for(var n in t||(t={}))Yt.call(t,n)&&qt(e,n,t[n]);if(Qt)for(var n of Qt(t))Xt.call(t,n)&&qt(e,n,t[n]);return e};const en=({onAction:e})=>{const t=(0,o.isWalmartSearchPage)(),n=(0,o.isWalmartProductPageNew)(),r=(0,D.useMemo)((()=>({isProductPage:n,isSearchPage:t})),[n,t]),a=(0,D.useMemo)((()=>({overrideAction:(t,n)=>{return r=void 0,o=null,a=function*(){return e(t,n)},new Promise(((e,t)=>{var n=e=>{try{l(a.next(e))}catch(e){t(e)}},i=e=>{try{l(a.throw(e))}catch(e){t(e)}},l=t=>t.done?e(t.value):Promise.resolve(t.value).then(n,i);l((a=a.apply(r,o)).next())}));var r,o,a},onPostClickAction:()=>ue()})),[e]),{features:i}=(0,ft.Q)();return(0,E.jsx)(tn,{children:(0,E.jsx)(nn,{children:t||n?(0,E.jsxs)(E.Fragment,{children:[(null==i?void 0:i.includes(ft.l.XRAY_WALMART))&&(0,E.jsx)(o.MenuXrayWalmartButton,Jt(Jt({},r),a)),n&&(null==i?void 0:i.includes(ft.l.CALCULATOR_WALMART))?(0,E.jsx)(o.MenuCalculatorWalmartButton,Jt(Jt({},r),a)):void 0,(null==i?void 0:i.includes(ft.l.FEEDBACK))&&(0,E.jsx)(o.MenuFeedbackButton,Jt({},a))]}):(null==i?void 0:i.includes(ft.l.XRAY_WALMART))&&(0,E.jsx)(bt,{})})})},tn=(0,B.default)(Se.Rm)`
  display: flex;
  flex-grow: 1;
`,nn=B.default.div`
  display: flex;
  width: 100%;
  flex-direction: column;
  gap: ${({theme:e})=>e.spacing.sp8};
  margin-bottom: ${({theme:e})=>e.spacing.sp8};
`,rn=e=>"amazon"===e.platform||"h10"===e.platform?(0,E.jsx)(Nt,{onAction:e.onAction,isSellerMode:e.isSellerMode}):"walmart"===e.platform?(0,E.jsx)(en,{onAction:e.onAction}):"tiktok"===e.platform?(0,E.jsx)(Vt,{onAction:e.onAction}):(0,E.jsx)(E.Fragment,{});var on,an,ln,sn,un=n(77062);function cn(){return cn=Object.assign?Object.assign.bind():function(e){for(var t=1;t<arguments.length;t++){var n=arguments[t];for(var r in n)({}).hasOwnProperty.call(n,r)&&(e[r]=n[r])}return e},cn.apply(null,arguments)}const dn=e=>D.createElement("svg",cn({xmlns:"http://www.w3.org/2000/svg",width:73,height:84,fill:"none"},e),on||(on=D.createElement("g",{filter:"url(#about-ext_svg__a)"},D.createElement("path",{fill:"#0A6FE8",d:"M8.5 62a6 6 0 0 0 6 6h50V16h-50a6 6 0 0 0-6 6v40Z"}))),an||(an=D.createElement("path",{fill:"#A6B7C9",d:"M64.5 0h1v84h-1z"})),ln||(ln=D.createElement("defs",null,D.createElement("filter",{id:"about-ext_svg__a",width:72,height:68,x:.5,y:9,colorInterpolationFilters:"sRGB",filterUnits:"userSpaceOnUse"},D.createElement("feFlood",{floodOpacity:0,result:"BackgroundImageFix"}),D.createElement("feColorMatrix",{in:"SourceAlpha",result:"hardAlpha",values:"0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 127 0"}),D.createElement("feOffset",{dy:1}),D.createElement("feGaussianBlur",{stdDeviation:4}),D.createElement("feColorMatrix",{values:"0 0 0 0 0.403922 0 0 0 0 0.517647 0 0 0 0 0.635294 0 0 0 0.16 0"}),D.createElement("feBlend",{in2:"BackgroundImageFix",result:"effect1_dropShadow_2493_74117"}),D.createElement("feColorMatrix",{in:"SourceAlpha",result:"hardAlpha",values:"0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 127 0"}),D.createElement("feOffset",{dy:2}),D.createElement("feGaussianBlur",{stdDeviation:1.5}),D.createElement("feColorMatrix",{values:"0 0 0 0 0.403922 0 0 0 0 0.517647 0 0 0 0 0.635294 0 0 0 0.08 0"}),D.createElement("feBlend",{in2:"effect1_dropShadow_2493_74117",result:"effect2_dropShadow_2493_74117"}),D.createElement("feBlend",{in:"SourceGraphic",in2:"effect2_dropShadow_2493_74117",result:"shape"})))),sn||(sn=D.createElement("path",{fill:"#fff",d:"M24.5 34.91v7.635h5.727v-4.459L24.5 34.91ZM33.773 38.086v4.46H39.5v-7.637l-5.727 3.177ZM42.773 33.152v9.393H48.5V30l-5.727 3.152ZM24.5 45.545h5.727v7.636H24.5zM33.773 45.545H39.5v7.636h-5.727zM42.773 45.545H48.5v7.636h-5.727z"})));var pn,gn;function hn(){return hn=Object.assign?Object.assign.bind():function(e){for(var t=1;t<arguments.length;t++){var n=arguments[t];for(var r in n)({}).hasOwnProperty.call(n,r)&&(e[r]=n[r])}return e},hn.apply(null,arguments)}const mn=e=>D.createElement("svg",hn({xmlns:"http://www.w3.org/2000/svg",width:268,height:124,fill:"none"},e),pn||(pn=D.createElement("g",{clipPath:"url(#welcome_svg__a)"},D.createElement("path",{fill:"#F2F5F7",d:"M80.259 117.941c13.695 0 24.797-11.099 24.797-24.79 0-13.691-11.102-24.79-24.797-24.79-13.696 0-24.798 11.099-24.798 24.79 0 13.691 11.102 24.79 24.798 24.79Z"}),D.createElement("path",{fill:"#F90",d:"M80.13 107.103c-5.356-.059-10.246-1.626-14.581-4.859a22.277 22.277 0 0 1-1.597-1.309c-.05-.049-.11-.099-.149-.158-.09-.119-.1-.258-.03-.367.07-.099.199-.149.348-.109a.86.86 0 0 1 .218.099 30.858 30.858 0 0 0 5.277 2.409 33.63 33.63 0 0 0 5.564 1.428c1.855.308 3.71.456 5.585.447a30.04 30.04 0 0 0 4.116-.298 32.97 32.97 0 0 0 7.608-2.003c.198-.079.387-.159.585-.238a.704.704 0 0 1 .377-.05c.179.03.317.119.397.288.07.169.03.337-.09.476-.069.089-.168.149-.257.218-1.389 1.002-2.897 1.765-4.504 2.38a24.979 24.979 0 0 1-5.306 1.368c-1.17.179-2.351.258-3.571.268l.01.01Z"}),D.createElement("path",{fill:"#F90",d:"M94.483 99.636c.516.02 1.041.04 1.557.179.09.02.169.04.258.07.377.128.466.257.496.654.04.694-.1 1.358-.298 2.013-.268.902-.664 1.735-1.27 2.469a5.132 5.132 0 0 1-.555.575.7.7 0 0 1-.248.149c-.178.049-.288-.04-.268-.228 0-.09.05-.169.08-.258.307-.774.605-1.557.823-2.36.09-.337.178-.675.178-1.022v-.238c0-.267-.138-.436-.386-.535-.288-.109-.596-.139-.903-.159-.774-.03-1.537.03-2.301.119-.248.03-.496.06-.754.08-.11 0-.218.009-.268-.11-.05-.119.01-.218.1-.307.059-.059.138-.109.208-.159.644-.416 1.349-.654 2.102-.793.477-.09.963-.119 1.459-.139h-.01Z"}),D.createElement("path",{fill:"#000",d:"M88.402 95.58c-.129-.208-.287-.406-.436-.604-.625-.823-.903-1.736-.893-2.767.02-2.112 0-4.224 0-6.336 0-.555-.02-1.11-.07-1.656-.178-2.112-1.24-3.59-3.213-4.333-2.619-.982-5.237-.893-7.787.218-1.716.743-2.826 2.072-3.322 3.877-.06.208-.11.426-.13.644-.029.308.11.486.388.615.119.05.257.06.386.08.953.099 1.915.208 2.867.307.446.05.684-.09.823-.516.04-.109.06-.228.09-.337.188-.625.555-1.12 1.13-1.438.705-.396 1.458-.486 2.242-.317.922.198 1.458.783 1.637 1.716.119.624.05 1.249.079 1.874 0 .188-.119.198-.258.208-.784.08-1.567.159-2.35.258-1.141.139-2.282.317-3.373.704-1.756.624-3.115 1.705-3.81 3.48-.564 1.458-.594 2.965-.138 4.453.516 1.685 1.666 2.727 3.363 3.173.684.178 1.378.188 2.083.168 1.577-.03 2.975-.525 4.166-1.567.327-.287.654-.584 1.021-.912.14.209.268.387.387.566.377.575.843 1.08 1.359 1.526.446.377.744.387 1.19 0 .804-.694 1.607-1.388 2.41-2.092.368-.317.407-.575.15-.991h.01Zm-6.209-5.235v.476c0 .674-.02 1.339-.189 1.993-.218.853-.605 1.607-1.299 2.172-.377.307-.813.515-1.31.555-1.17.11-2.023-.535-2.27-1.696-.12-.565-.12-1.13.01-1.685.227-.982.832-1.646 1.735-2.043.794-.347 1.647-.486 2.5-.516.258 0 .575-.138.754.04.158.159.04.466.05.714l.019-.01Z"}),D.createElement("path",{fill:"#5CB3FF",d:"M70.766 53.14c11.055 0 20.017-8.959 20.017-20.01 0-11.052-8.962-20.011-20.017-20.011S50.75 22.078 50.75 33.129c0 11.052 8.962 20.011 20.016 20.011Z"}),D.createElement("path",{fill:"#fff",d:"M70.766 22.837c-5.673 0-10.296 4.62-10.296 10.293 0 5.671 4.623 10.292 10.296 10.292 5.674 0 10.296-4.62 10.296-10.292 0-5.673-4.622-10.293-10.296-10.293Zm0 19.276c-4.96 0-8.986-4.026-8.986-8.983 0-4.959 4.037-8.985 8.986-8.985 4.95 0 8.987 4.026 8.987 8.985 0 4.957-4.027 8.983-8.987 8.983Z"}),D.createElement("path",{fill:"#fff",d:"M73.355 34.954a4.468 4.468 0 0 0 .992-2.796 4.537 4.537 0 0 0-4.533-4.532 4.537 4.537 0 0 0-4.533 4.532 4.537 4.537 0 0 0 4.533 4.531 4.524 4.524 0 0 0 2.768-.962l2.747 2.737a.548.548 0 0 0 .774 0 .55.55 0 0 0 0-.783l-2.748-2.737v.01Zm-.109-2.796a3.433 3.433 0 0 1-6.864 0 3.433 3.433 0 0 1 6.864 0Z"}),D.createElement("path",{fill:"#F2F5F7",d:"M235.759 84.693c17.53 0 31.741-14.207 31.741-31.731 0-17.525-14.211-31.732-31.741-31.732s-31.741 14.207-31.741 31.732c0 17.524 14.211 31.73 31.741 31.73Z"}),D.createElement("path",{fill:"#3C9",d:"M231.296 64.583s-9.741-14.646-8.005-16.381c1.736-1.735 7.132 12.038 7.132 12.038s-6.279-21.696-4.494-23.491c1.786-1.795 6.954 20.298 6.954 20.298s-.447-24.503 1.785-24.503c2.688 0-.268 22.907 0 24.503.258 1.596 4.761-23.63 7.37-23.63s-4.355 23.055-4.642 24.066c-.288 1.012 8.996-24.353 9.423-21.894.436 2.469-7.687 24.215-7.687 24.215s7.727-18.107 9.423-16.411c1.706 1.705-7.251 16.55-7.251 16.55s7.181-9.203 8.262-8.122c1.082 1.081-8.262 12.762-8.262 12.762h-10.008Z"}),D.createElement("path",{fill:"#0A6FE8",d:"M239.092 56.214h-19.055c-.863 0-1.497.942-1.358 1.973.615 4.413 3.174 14.726 13.589 15.192h6.963c10.415-.466 12.974-10.789 13.589-15.192.149-1.031-.496-1.973-1.359-1.973h-12.389.02Z"}),D.createElement("path",{fill:"#E8F6FF",d:"M145.675 87.261c24.103 0 43.643-19.534 43.643-43.63C189.318 19.533 169.778 0 145.675 0c-24.104 0-43.644 19.534-43.644 43.63 0 24.097 19.54 43.631 43.644 43.631Z"}),D.createElement("path",{fill:"#0A6FE8",d:"M123.278 28.826v14.17h10.811v-8.26l-10.811-5.91ZM140.269 34.736v8.26h10.722v-14.17l-10.722 5.91ZM157.26 25.474v17.522h10.713V19.564l-10.713 5.91Z"}),D.createElement("path",{fill:"#003873",d:"M134.089 48.718h-10.811v14.17h10.811v-14.17ZM151.081 48.718h-10.812v14.17h10.812v-14.17ZM168.072 48.718H157.26v14.17h10.812v-14.17Z"}),D.createElement("path",{fill:"#E8F6FF",d:"M23.81 88.917c12.875 0 23.31-10.432 23.31-23.303 0-12.87-10.435-23.302-23.31-23.302C10.935 42.312.5 52.743.5 65.614s10.435 23.303 23.31 23.303Z"}),D.createElement("path",{fill:"#E8C328",d:"M35.415 69.541v-.02H22.669c-.645-2.558-2.956-4.462-5.703-4.462a5.898 5.898 0 0 0-5.892 5.89 5.892 5.892 0 0 0 5.892 5.89c2.876 0 5.267-2.072 5.782-4.799h6.08v4.274h2.52V72.04h2.659v4.274h2.519V69.54h-1.12.009Zm-18.45 4.78a3.376 3.376 0 0 1-3.372-3.372 3.37 3.37 0 0 1 3.373-3.371 3.37 3.37 0 0 1 3.372 3.371 3.37 3.37 0 0 1-3.372 3.372Z"}),D.createElement("path",{fill:"#5CB3FF",d:"M25.545 55.361H12.661a.832.832 0 0 0-.834.833c0 .466.377.833.834.833h12.884a.832.832 0 0 0 .834-.833.832.832 0 0 0-.834-.833ZM34.87 55.361h-5.545a.832.832 0 0 0-.834.833c0 .466.377.833.834.833h5.544a.832.832 0 0 0 .834-.833.832.832 0 0 0-.834-.833ZM34.88 64.573H23.62a.832.832 0 0 0-.833.833c0 .466.377.833.833.833H34.88a.832.832 0 1 0 0-1.666ZM34.959 59.972H20.864a.832.832 0 0 0-.834.833c0 .466.377.833.834.833h14.095a.832.832 0 0 0 .833-.833.832.832 0 0 0-.833-.833ZM17.174 59.972h-4.305a.832.832 0 0 0-.833.833c0 .466.377.833.833.833h4.305a.832.832 0 0 0 .833-.833.832.832 0 0 0-.833-.833Z"}),D.createElement("path",{fill:"#A5DBFF",d:"M190.846 124c10.956 0 19.838-8.879 19.838-19.832 0-10.953-8.882-19.832-19.838-19.832-10.956 0-19.838 8.879-19.838 19.832 0 10.953 8.882 19.832 19.838 19.832Z"}),D.createElement("path",{fill:"#fff",d:"M190.846 116.325c6.458 0 11.694-5.234 11.694-11.691 0-6.457-5.236-11.691-11.694-11.691-6.459 0-11.695 5.234-11.695 11.691 0 6.457 5.236 11.691 11.695 11.691Z"}),D.createElement("path",{fill:"#F64F45",d:"M190.846 97.96c3.68 0 6.675 2.995 6.675 6.674 0 3.679-2.995 6.673-6.675 6.673-3.68 0-6.676-2.994-6.676-6.673s2.996-6.674 6.676-6.674Zm0-1.844a8.529 8.529 0 1 0 0 17.056 8.529 8.529 0 1 0 0-17.056Z"}),D.createElement("path",{fill:"#F64F45",d:"M190.846 93.974c2.846 0 5.525 1.11 7.538 3.124a10.588 10.588 0 0 1 3.125 7.536c0 2.846-1.111 5.523-3.125 7.536a10.596 10.596 0 0 1-7.538 3.124 10.598 10.598 0 0 1-7.539-3.124 10.591 10.591 0 0 1-3.124-7.536c0-2.846 1.111-5.523 3.124-7.536a10.598 10.598 0 0 1 7.539-3.124Zm0-1.993c-6.983 0-12.647 5.662-12.647 12.643 0 6.981 5.664 12.643 12.647 12.643 6.983 0 12.646-5.662 12.646-12.643 0-6.98-5.663-12.643-12.646-12.643Z"}),D.createElement("path",{fill:"#F64F45",d:"M190.846 108.739a4.105 4.105 0 1 0 0-8.21 4.106 4.106 0 1 0 0 8.21Z"}),D.createElement("path",{fill:"#0253B6",d:"M201.628 94.034V91.95l-4.017 4.026v.913l-7.212 7.199a.838.838 0 0 0-.238.575c0 .218.09.426.238.585a.836.836 0 0 0 .576.238.836.836 0 0 0 .575-.238l7.201-7.2h.923l4.027-4.035h-2.083l.01.02Z"}),D.createElement("path",{fill:"#3C9",d:"M70.836 52.981c10.956 0 19.838-8.879 19.838-19.832 0-10.953-8.882-19.832-19.838-19.832-10.956 0-19.838 8.88-19.838 19.832 0 10.953 8.882 19.832 19.838 19.832Z"}),D.createElement("path",{fill:"#fff",d:"M70.836 22.946c-5.624 0-10.207 4.571-10.207 10.203 0 5.633 4.573 10.204 10.207 10.204s10.207-4.571 10.207-10.204c0-5.632-4.573-10.203-10.207-10.203Zm0 19.108c-4.91 0-8.907-3.996-8.907-8.905 0-4.908 3.997-8.904 8.907-8.904s8.907 3.996 8.907 8.904c0 4.909-3.997 8.905-8.907 8.905Z"}),D.createElement("path",{fill:"#fff",stroke:"#fff",strokeMiterlimit:10,strokeWidth:.05,d:"M69.883 36.66a4.47 4.47 0 0 1-4.463-4.463 4.47 4.47 0 0 1 4.463-4.462 4.47 4.47 0 0 1 4.464 4.462 4.47 4.47 0 0 1-4.463 4.463Zm0-7.894a3.433 3.433 0 0 0-3.431 3.431 3.433 3.433 0 0 0 6.864 0 3.433 3.433 0 0 0-3.433-3.43Z"}),D.createElement("path",{fill:"#fff",stroke:"#fff",strokeMiterlimit:10,strokeWidth:.05,d:"M75.736 38.573a.526.526 0 0 1-.367-.148l-2.976-2.955a.527.527 0 0 1 0-.734.527.527 0 0 1 .734 0l2.976 2.955a.527.527 0 0 1 0 .734c-.1.099-.238.148-.367.148Z"}))),gn||(gn=D.createElement("defs",null,D.createElement("clipPath",{id:"welcome_svg__a"},D.createElement("path",{fill:"#fff",d:"M.5 0h267v124H.5z"}))))),fn=()=>{const{t:e}=(0,o.useTranslation)();return(0,E.jsxs)(vn,{children:[(0,E.jsx)(kn,{children:e("onboarding.welcomeHeader")}),(0,E.jsx)(mn,{}),(0,E.jsx)(yn,{children:e("onboarding.welcomeText")})]})},vn=B.default.div`
  display: flex;
  gap: ${({theme:e})=>e.spacing.sp24};
  flex-direction: column;
  align-items: center;
  padding-top: ${({theme:e})=>e.spacing.sp24};
`,yn=B.default.p`
  ${Ie.Qv};
  text-align: center;
  color: ${({theme:e})=>e.systemColors.grey800};
  max-width: 300px;
`,kn=B.default.h2`
  ${Ie.f};
  color: ${({theme:e})=>e.systemColors.blue600};
  text-align: center;
  margin: 0 auto;
`,xn=()=>{const{t:e}=(0,o.useTranslation)();return(0,E.jsxs)(wn,{children:[(0,E.jsx)(kn,{children:e("onboarding.aboutHeader")}),(0,E.jsx)(dn,{}),(0,E.jsx)(yn,{children:e("onboarding.aboutText")})]})},wn=B.default.div`
  display: flex;
  gap: ${({theme:e})=>e.spacing.sp24};
  flex-direction: column;
  align-items: center;
  padding-top: ${({theme:e})=>e.spacing.sp24};
`;var bn,Cn,Mn,Ln,Sn,jn,In,Tn,Pn,En,Dn,On,An,Rn,Nn,Bn,$n,Gn,Fn,zn,Un,_n,Wn,Vn,Hn;function Zn(){return Zn=Object.assign?Object.assign.bind():function(e){for(var t=1;t<arguments.length;t++){var n=arguments[t];for(var r in n)({}).hasOwnProperty.call(n,r)&&(e[r]=n[r])}return e},Zn.apply(null,arguments)}const Kn=e=>D.createElement("svg",Zn({xmlns:"http://www.w3.org/2000/svg",width:312,height:176,fill:"none"},e),bn||(bn=D.createElement("rect",{width:311,height:135,x:.5,y:40.5,fill:"#fff",rx:5.5})),Cn||(Cn=D.createElement("path",{fill:"#13191F",d:"M17.392 73V61.72h7.12v1.104h-5.84v3.904h5.504v1.104h-5.504v4.064h5.84V73h-7.12Zm8.16 0 3.232-4.016-3.04-3.76h1.536l2.256 2.864 2.272-2.864h1.52l-3.04 3.76L33.52 73h-1.536l-2.464-3.088L27.088 73h-1.536Zm12.518.144c-.917 0-1.605-.24-2.064-.72-.458-.49-.688-1.195-.688-2.112v-4.08h-1.52v-1.008h1.52V62.84h1.296v2.384h2.464v1.008h-2.464v3.952c0 .608.128 1.072.384 1.392.256.31.672.464 1.248.464.171 0 .342-.021.512-.064.171-.043.326-.085.464-.128l.224.992a2.631 2.631 0 0 1-.608.208 3.29 3.29 0 0 1-.768.096Zm5.9 0c-1.227 0-2.197-.357-2.912-1.072-.715-.725-1.072-1.707-1.072-2.944 0-.8.16-1.504.48-2.112.32-.619.757-1.093 1.312-1.424.565-.341 1.216-.512 1.952-.512 1.056 0 1.883.341 2.48 1.024.597.672.896 1.6.896 2.784v.496H41.25c.043.885.299 1.563.768 2.032.47.459 1.12.688 1.952.688.47 0 .917-.07 1.344-.208.427-.15.832-.39 1.216-.72l.448.912c-.352.33-.8.592-1.344.784a5.23 5.23 0 0 1-1.664.272Zm-.208-7.104c-.736 0-1.317.23-1.744.688-.427.459-.677 1.061-.752 1.808h4.72c-.032-.79-.24-1.403-.624-1.84-.373-.437-.907-.656-1.6-.656Zm5.22 6.96v-7.776h1.264v1.328a2.53 2.53 0 0 1 1.088-1.104c.469-.245.997-.368 1.584-.368 1.845 0 2.768 1.024 2.768 3.072V73H54.39v-4.768c0-.725-.144-1.253-.432-1.584-.278-.341-.726-.512-1.344-.512-.715 0-1.286.224-1.712.672-.416.437-.624 1.024-.624 1.76V73h-1.296Zm11.717.144a5.739 5.739 0 0 1-1.728-.256c-.544-.17-.992-.41-1.344-.72l.416-.912c.384.31.806.539 1.264.688a5 5 0 0 0 1.424.208c.608 0 1.067-.112 1.376-.336.31-.224.464-.528.464-.912 0-.299-.101-.533-.304-.704-.202-.181-.522-.32-.96-.416l-1.456-.32c-1.322-.288-1.984-.97-1.984-2.048 0-.704.278-1.27.832-1.696.566-.427 1.302-.64 2.208-.64.534 0 1.04.085 1.52.256.491.16.896.4 1.216.72l-.432.912a3.127 3.127 0 0 0-1.088-.656c-.405-.16-.81-.24-1.216-.24-.597 0-1.05.117-1.36.352-.309.235-.464.544-.464.928 0 .597.395.981 1.184 1.152l1.456.304c.683.15 1.2.39 1.552.72.352.32.528.757.528 1.312 0 .715-.282 1.28-.848 1.696-.565.405-1.317.608-2.256.608Zm4.92-9.856v-1.456h1.6v1.456h-1.6Zm.16 9.712v-7.776h1.296V73h-1.297Zm6.95.144c-.757 0-1.413-.165-1.968-.496a3.405 3.405 0 0 1-1.296-1.392c-.298-.608-.448-1.323-.448-2.144 0-.821.15-1.53.448-2.128a3.382 3.382 0 0 1 1.296-1.408c.555-.33 1.21-.496 1.968-.496.747 0 1.398.165 1.952.496.566.33.998.8 1.296 1.408.31.597.464 1.307.464 2.128 0 .821-.154 1.536-.464 2.144a3.27 3.27 0 0 1-1.296 1.392c-.554.33-1.205.496-1.952.496Zm0-1.04c.726 0 1.307-.256 1.744-.768.438-.523.656-1.264.656-2.224 0-.97-.218-1.712-.656-2.224-.437-.512-1.018-.768-1.744-.768-.736 0-1.322.256-1.76.768-.426.512-.64 1.253-.64 2.224 0 .96.214 1.701.64 2.224.438.512 1.024.768 1.76.768Zm5.659.896v-7.776h1.264v1.328a2.53 2.53 0 0 1 1.088-1.104c.47-.245.997-.368 1.584-.368 1.845 0 2.768 1.024 2.768 3.072V73h-1.296v-4.768c0-.725-.144-1.253-.432-1.584-.278-.341-.726-.512-1.344-.512-.715 0-1.285.224-1.712.672-.416.437-.624 1.024-.624 1.76V73h-1.296Zm11.718.144a5.739 5.739 0 0 1-1.728-.256c-.544-.17-.992-.41-1.344-.72l.416-.912c.384.31.805.539 1.264.688a5 5 0 0 0 1.424.208c.608 0 1.066-.112 1.376-.336.309-.224.464-.528.464-.912 0-.299-.102-.533-.304-.704-.203-.181-.523-.32-.96-.416l-1.456-.32c-1.323-.288-1.984-.97-1.984-2.048 0-.704.277-1.27.832-1.696.565-.427 1.301-.64 2.208-.64.533 0 1.04.085 1.52.256.49.16.896.4 1.216.72l-.432.912a3.127 3.127 0 0 0-1.088-.656 3.29 3.29 0 0 0-1.216-.24c-.598 0-1.051.117-1.36.352a1.11 1.11 0 0 0-.464.928c0 .597.394.981 1.184 1.152l1.456.304c.682.15 1.2.39 1.552.72.352.32.528.757.528 1.312 0 .715-.283 1.28-.848 1.696-.566.405-1.318.608-2.256.608Z"})),Mn||(Mn=D.createElement("path",{fill:"#0081FF",d:"M16 98.09v6.365h4.773v-3.717L16 98.091ZM23.727 100.738v3.717H28.5v-6.364l-4.773 2.647ZM31.227 96.627v7.828H36V94l-4.773 2.627Z"})),Ln||(Ln=D.createElement("path",{fill:"#003873",d:"M16 106.955h4.773v6.364H16zM23.727 106.955H28.5v6.364h-4.773zM31.227 106.955H36v6.364h-4.773z"})),Sn||(Sn=D.createElement("path",{fill:"#13191F",d:"M53.218 109v-9.87h1.148v4.368h5.922V99.13h1.148V109h-1.148v-4.508h-5.922V109h-1.148Zm13.562.126c-1.073 0-1.922-.313-2.548-.938-.625-.635-.938-1.493-.938-2.576 0-.7.14-1.316.42-1.848.28-.541.663-.957 1.148-1.246.495-.299 1.064-.448 1.708-.448.924 0 1.648.299 2.17.896.523.588.784 1.4.784 2.436v.434H64.4c.038.775.262 1.367.672 1.778.411.401.98.602 1.708.602.411 0 .803-.061 1.176-.182a3.315 3.315 0 0 0 1.064-.63l.392.798c-.308.289-.7.518-1.176.686a4.566 4.566 0 0 1-1.456.238Zm-.182-6.216c-.644 0-1.152.201-1.526.602-.373.401-.592.929-.658 1.582h4.13c-.028-.691-.21-1.227-.546-1.61-.326-.383-.793-.574-1.4-.574Zm6.654 6.216c-.691 0-1.214-.201-1.568-.602-.346-.411-.518-1.003-.518-1.778V99.13H72.3v7.532c0 .999.396 1.498 1.19 1.498.233 0 .443-.028.63-.084l-.028.952c-.29.065-.57.098-.84.098Zm1.916-8.624v-1.274h1.4v1.274h-1.4Zm.14 8.498v-6.804h1.134V109h-1.134Zm5.662.126c-1.633 0-2.45-.901-2.45-2.702v-4.228h1.134v4.214c0 .607.121 1.055.364 1.344.252.289.644.434 1.176.434.579 0 1.05-.191 1.414-.574.364-.392.546-.91.546-1.554v-3.864h1.134V109h-1.106v-1.148a2.294 2.294 0 0 1-.91.952 2.611 2.611 0 0 1-1.302.322Zm5.468-.126v-6.804h1.106v1.148c.205-.411.49-.723.854-.938.364-.224.788-.336 1.274-.336 1.064 0 1.745.462 2.044 1.386.205-.429.508-.765.91-1.008a2.551 2.551 0 0 1 1.386-.378c1.512 0 2.268.896 2.268 2.688V109h-1.134v-4.186c0-.625-.112-1.083-.336-1.372-.215-.299-.574-.448-1.078-.448-.551 0-.99.196-1.316.588-.327.392-.49.915-.49 1.568V109h-1.134v-4.186c0-.625-.112-1.083-.336-1.372-.215-.299-.574-.448-1.078-.448-.56 0-1.004.196-1.33.588-.318.392-.476.915-.476 1.568V109h-1.134Zm16.23 0v-.98h2.254v-7.504l-1.778 1.12-.49-.854 2.59-1.652h.826v8.89h2.114v.98h-5.516Zm10.858.126c-1.129 0-1.988-.434-2.576-1.302-.588-.868-.882-2.123-.882-3.766 0-1.652.294-2.907.882-3.766.588-.859 1.447-1.288 2.576-1.288 1.139 0 1.998.43 2.576 1.288.588.849.882 2.1.882 3.752 0 1.652-.298 2.912-.896 3.78-.588.868-1.442 1.302-2.562 1.302Zm0-.994c.784 0 1.363-.331 1.736-.994.383-.663.574-1.694.574-3.094s-.186-2.422-.56-3.066c-.373-.653-.956-.98-1.75-.98-.784 0-1.367.327-1.75.98-.373.653-.56 1.675-.56 3.066 0 1.4.187 2.431.56 3.094.383.663.966.994 1.75.994Z"})),jn||(jn=D.createElement("rect",{width:32,height:32,x:216,y:88,fill:"#D9E0E8",rx:6})),D.createElement("mask",{id:"favoriting-ext_svg__b",width:16,height:16,x:224,y:96,maskUnits:"userSpaceOnUse",style:{maskType:"alpha"}},In||(In=D.createElement("g",{clipPath:"url(#favoriting-ext_svg__a)"},D.createElement("path",{fill:"#485E75",d:"M231.25 107H227a.997.997 0 0 1-.97-1.244l.195-.778a5.241 5.241 0 0 1 2.148-3.075l.339-4.403h-.962a.747.747 0 0 1-.75-.75.75.75 0 0 1 .75-.75h8.5a.749.749 0 1 1 0 1.5h-.962l.34 4.403a5.244 5.244 0 0 1 2.147 3.075l.194.778A1.001 1.001 0 0 1 237 107h-4.25v4.25c0 .416-.334.75-.778.75a.766.766 0 0 1-.75-.75l.028-4.25Zm-1.437-4.259-.597.406a3.726 3.726 0 0 0-1.536 2.194l-.04.159h3.61v-2.75c0-.416.334-.75.75-.75s.75.334.75.75v2.75h3.609l-.04-.159c-.228-.922-.816-1.691-1.535-2.194l-.625-.406-.375-5.241h-3.568l-.403 5.241Z"})))),Tn||(Tn=D.createElement("g",{mask:"url(#favoriting-ext_svg__b)"},D.createElement("path",{fill:"#485E75",d:"M222 94h20v20h-20z"}))),D.createElement("mask",{id:"favoriting-ext_svg__c",width:16,height:16,x:264,y:96,maskUnits:"userSpaceOnUse",style:{maskType:"alpha"}},Pn||(Pn=D.createElement("path",{fill:"#485E75",d:"M272 107.5a1.5 1.5 0 1 1-.001 3.001A1.5 1.5 0 0 1 272 107.5Zm0-5a1.5 1.5 0 1 1-.001 3.001A1.5 1.5 0 0 1 272 102.5Zm0-2a1.5 1.5 0 1 1 .001-3.001A1.5 1.5 0 0 1 272 100.5Z"}))),En||(En=D.createElement("g",{mask:"url(#favoriting-ext_svg__c)"},D.createElement("path",{fill:"#485E75",d:"M262 94h20v20h-20z"}))),Dn||(Dn=D.createElement("g",{filter:"url(#favoriting-ext_svg__d)"},D.createElement("path",{fill:"#fff",d:"M239.27 119.28c-.28-.36-.63-1.09-1.24-2-.35-.5-1.21-1.45-1.47-1.94a1.257 1.257 0 0 1-.15-1 1.32 1.32 0 0 1 1.43-1c.511.103.98.353 1.35.72.258.243.496.507.71.79.16.2.2.28.38.51.18.23.3.46.21.12-.07-.5-.19-1.34-.36-2.09-.13-.57-.16-.66-.28-1.09-.12-.43-.19-.79-.32-1.28a14.184 14.184 0 0 1-.28-1.46 2.823 2.823 0 0 1 .26-1.85 1.19 1.19 0 0 1 1.3-.22c.44.325.769.78.94 1.3.262.64.437 1.313.52 2 .16 1 .47 2.46.48 2.76 0-.37-.07-1.15 0-1.5.069-.365.323-.668.67-.8.298-.091.613-.112.92-.06.31.065.585.243.77.5.232.583.36 1.203.38 1.83a6.758 6.758 0 0 1 .28-1.62c.167-.235.411-.405.69-.48a2.8 2.8 0 0 1 1 0c.271.091.508.262.68.49.212.53.34 1.09.38 1.66 0 .14.07-.39.29-.74a1.004 1.004 0 0 1 1.9.64v2.26a11.86 11.86 0 0 1-.24 1.74 5.906 5.906 0 0 1-.72 1.43 7.188 7.188 0 0 0-1.19 1.81 4.006 4.006 0 0 0-.1 1c-.001.311.039.62.12.92a5.88 5.88 0 0 1-1.23 0c-.39-.06-.87-.84-1-1.08a.38.38 0 0 0-.68 0c-.22.38-.71 1.07-1.05 1.11-.67.08-2.05 0-3.14 0 0 0 .18-1-.23-1.36-.41-.36-.83-.78-1.14-1.06l-.84-.96Z"}),D.createElement("path",{stroke:"#000",strokeLinecap:"round",strokeLinejoin:"round",strokeWidth:.75,d:"M239.27 119.28c-.28-.36-.63-1.09-1.24-2-.35-.5-1.21-1.45-1.47-1.94a1.257 1.257 0 0 1-.15-1 1.32 1.32 0 0 1 1.43-1c.511.103.98.353 1.35.72.258.243.496.507.71.79.16.2.2.28.38.51.18.23.3.46.21.12-.07-.5-.19-1.34-.36-2.09-.13-.57-.16-.66-.28-1.09-.12-.43-.19-.79-.32-1.28a14.184 14.184 0 0 1-.28-1.46 2.823 2.823 0 0 1 .26-1.85 1.19 1.19 0 0 1 1.3-.22c.44.325.769.78.94 1.3.262.64.437 1.313.52 2 .16 1 .47 2.46.48 2.76 0-.37-.07-1.15 0-1.5.069-.365.323-.668.67-.8.298-.091.613-.112.92-.06.31.065.585.243.77.5.232.583.36 1.203.38 1.83a6.758 6.758 0 0 1 .28-1.62c.167-.235.411-.405.69-.48a2.8 2.8 0 0 1 1 0c.271.091.508.262.68.49.212.53.34 1.09.38 1.66 0 .14.07-.39.29-.74a1.004 1.004 0 0 1 1.9.64v2.26a11.86 11.86 0 0 1-.24 1.74 5.906 5.906 0 0 1-.72 1.43 7.188 7.188 0 0 0-1.19 1.81 4.006 4.006 0 0 0-.1 1c-.001.311.039.62.12.92a5.88 5.88 0 0 1-1.23 0c-.39-.06-.87-.84-1-1.08a.38.38 0 0 0-.68 0c-.22.38-.71 1.07-1.05 1.11-.67.08-2.05 0-3.14 0 0 0 .18-1-.23-1.36-.41-.36-.83-.78-1.14-1.06l-.84-.96Z",clipRule:"evenodd"}),D.createElement("path",{fill:"#000",d:"M247.75 119.826v-3.452a.374.374 0 0 0-.75 0v3.452a.374.374 0 0 0 .75 0ZM245.77 119.825l-.02-3.454a.374.374 0 0 0-.377-.371.374.374 0 0 0-.373.375l.02 3.454c.001.206.17.372.377.371a.374.374 0 0 0 .373-.375ZM243 116.379l.02 3.446c.001.208.17.376.377.375a.376.376 0 0 0 .373-.379l-.02-3.446a.376.376 0 0 0-.377-.375.376.376 0 0 0-.373.379Z"}))),On||(On=D.createElement("rect",{width:20,height:20,x:16,y:134,fill:"#D9E0E8",rx:4})),An||(An=D.createElement("rect",{width:66,height:18,x:52,y:135,fill:"#D9E0E8",rx:4})),D.createElement("mask",{id:"favoriting-ext_svg__f",width:16,height:16,x:224,y:136,maskUnits:"userSpaceOnUse",style:{maskType:"alpha"}},Rn||(Rn=D.createElement("g",{clipPath:"url(#favoriting-ext_svg__e)"},D.createElement("path",{fill:"#485E75",d:"M231.25 147H227a.997.997 0 0 1-.97-1.244l.195-.778a5.241 5.241 0 0 1 2.148-3.075l.339-4.403h-.962a.75.75 0 1 1 0-1.5h8.5a.749.749 0 1 1 0 1.5h-.962l.34 4.403a5.244 5.244 0 0 1 2.147 3.075l.194.778A1.001 1.001 0 0 1 237 147h-4.25v4.25c0 .416-.334.75-.778.75a.766.766 0 0 1-.75-.75l.028-4.25Zm-1.437-4.259-.597.406a3.726 3.726 0 0 0-1.536 2.194l-.04.159h3.61v-2.75c0-.416.334-.75.75-.75s.75.334.75.75v2.75h3.609l-.04-.159c-.228-.922-.816-1.691-1.535-2.194l-.625-.406-.375-5.241h-3.568l-.403 5.241Z"})))),Nn||(Nn=D.createElement("g",{mask:"url(#favoriting-ext_svg__f)"},D.createElement("path",{fill:"#A6B7C9",d:"M222 134h20v20h-20z"}))),D.createElement("mask",{id:"favoriting-ext_svg__g",width:16,height:16,x:264,y:136,maskUnits:"userSpaceOnUse",style:{maskType:"alpha"}},Bn||(Bn=D.createElement("path",{fill:"#485E75",d:"M272 147.5a1.5 1.5 0 1 1-.001 3.001A1.5 1.5 0 0 1 272 147.5Zm0-5a1.5 1.5 0 1 1-.001 3.001A1.5 1.5 0 0 1 272 142.5Zm0-2a1.5 1.5 0 1 1 .001-3.001A1.5 1.5 0 0 1 272 140.5Z"}))),$n||($n=D.createElement("g",{mask:"url(#favoriting-ext_svg__g)"},D.createElement("path",{fill:"#A6B7C9",d:"M262 134h20v20h-20z"}))),Gn||(Gn=D.createElement("rect",{width:311,height:135,x:.5,y:40.5,stroke:"#D9E0E8",rx:5.5})),Fn||(Fn=D.createElement("circle",{cx:208,cy:16,r:12,fill:"#F2F2F2"})),zn||(zn=D.createElement("circle",{cx:236,cy:16,r:12,fill:"#F2F2F2"})),Un||(Un=D.createElement("g",{clipPath:"url(#favoriting-ext_svg__h)"},D.createElement("path",{fill:"#5F6467",d:"M270 16.75c0 .828-.504 1.5-1.125 1.5-.76 0-.743-.75-1.304-.75a.57.57 0 0 0-.571.572v3.178a.75.75 0 0 1-.75.75h-1.679a.57.57 0 0 1-.571-.572c0-.574.75-.544.75-1.303 0-.621-.671-1.125-1.5-1.125s-1.5.504-1.5 1.125c0 .76.75.743.75 1.304a.57.57 0 0 1-.572.571h-3.178a.75.75 0 0 1-.75-.75v-3.178c0-.316.256-.572.571-.572.555 0 .545.75 1.304.75.621 0 1.125-.673 1.125-1.5s-.504-1.521-1.125-1.521c-.76 0-.743.75-1.304.75a.554.554 0 0 1-.571-.55v-1.68c0-.414.336-.729.75-.729h3.178c.317-.041.572-.275.572-.592 0-.555-.75-.545-.75-1.304 0-.621.671-1.104 1.5-1.104s1.5.504 1.5 1.104c0 .76-.75.743-.75 1.304 0 .315.256.571.571.571h1.679c.414 0 .75.336.75.729v1.679c0 .315.256.571.571.571.555 0 .545-.75 1.304-.75.621 0 1.125.695 1.125 1.522Z"}))),_n||(_n=D.createElement("rect",{width:87,height:31,x:192.5,y:.5,stroke:"#F2F2F2",rx:15.5})),D.createElement("mask",{id:"favoriting-ext_svg__i",width:16,height:16,x:288,y:8,maskUnits:"userSpaceOnUse",style:{maskType:"alpha"}},Wn||(Wn=D.createElement("path",{fill:"#485E75",d:"M296 19.5a1.5 1.5 0 1 1-.001 3.001A1.5 1.5 0 0 1 296 19.5Zm0-5a1.5 1.5 0 1 1-.001 3.001A1.5 1.5 0 0 1 296 14.5Zm0-2a1.5 1.5 0 1 1 .001-3.001A1.5 1.5 0 0 1 296 12.5Z"}))),Vn||(Vn=D.createElement("g",{mask:"url(#favoriting-ext_svg__i)"},D.createElement("path",{fill:"#B3B3B3",d:"M286 6h20v20h-20z"}))),Hn||(Hn=D.createElement("defs",null,D.createElement("clipPath",{id:"favoriting-ext_svg__a"},D.createElement("path",{fill:"#fff",d:"M226 96h12v16h-12z"})),D.createElement("clipPath",{id:"favoriting-ext_svg__e"},D.createElement("path",{fill:"#fff",d:"M226 136h12v16h-12z"})),D.createElement("clipPath",{id:"favoriting-ext_svg__h"},D.createElement("path",{fill:"#fff",d:"M258 10h12v12h-12z"})),D.createElement("filter",{id:"favoriting-ext_svg__d",width:27.6,height:27.6,x:229.2,y:102.2,colorInterpolationFilters:"sRGB",filterUnits:"userSpaceOnUse"},D.createElement("feFlood",{floodOpacity:0,result:"BackgroundImageFix"}),D.createElement("feColorMatrix",{in:"SourceAlpha",result:"hardAlpha",values:"0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 127 0"}),D.createElement("feOffset",{dy:1}),D.createElement("feGaussianBlur",{stdDeviation:.9}),D.createElement("feColorMatrix",{values:"0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0.65 0"}),D.createElement("feBlend",{in2:"BackgroundImageFix",result:"effect1_dropShadow_2493_74904"}),D.createElement("feBlend",{in:"SourceGraphic",in2:"effect1_dropShadow_2493_74904",result:"shape"}))))),Qn=()=>{const{t:e}=(0,o.useTranslation)();return(0,E.jsxs)(Yn,{children:[(0,E.jsx)(kn,{children:e("onboarding.favoritingHeader")}),(0,E.jsx)(yn,{children:e("onboarding.favoritingText")}),(0,E.jsx)(Kn,{})]})},Yn=B.default.div`
  display: flex;
  gap: ${({theme:e})=>e.spacing.sp24};
  flex-direction: column;
  align-items: center;
  padding-top: ${({theme:e})=>e.spacing.sp24};
`,Xn=()=>{const{t:e}=(0,o.useTranslation)();return(0,E.jsxs)(qn,{children:[(0,E.jsx)(kn,{children:e("onboarding.startHeader")}),(0,E.jsx)(yn,{children:e("onboarding.startText")}),(0,E.jsx)(bt,{fromOnboarding:!0})]})},qn=B.default.div`
  display: flex;
  gap: ${({theme:e})=>e.spacing.sp24};
  flex-direction: column;
  align-items: center;
  padding-top: ${({theme:e})=>e.spacing.sp24};
`,Jn={1:"welcomePageNext",2:"widgetPageNext",3:"favoriteNext"},er={1:"welcomePageDismissed",2:"widgetPageDismissed",3:"favoriteDismissed"},tr=()=>{const{t:e}=(0,o.useTranslation)(),{onboardingStep:t}=(0,o.useGetStore)("local","onboardingStep"),n=(0,D.useMemo)((()=>t||1),[t]),r=(0,D.useMemo)((()=>[{title:e("onboarding.steps.welcome")},{title:e("onboarding.steps.about")},{title:e("onboarding.steps.favoriting")},{title:e("onboarding.steps.start")}]),[e]),a=(0,D.useCallback)((()=>{Jn[n]&&(0,o.sendSegmentEventMessage)({name:o.segmentEventType.onboardingStepCompleted,properties:{stepCompleted:Jn[n]}}).then(),3===n?ne.getState().finishOnboarding.action(!0):(0,o.setStore)("local",{onboardingStep:n+1}).then()}),[n]),i=(0,D.useCallback)((()=>{er[n]&&(0,o.sendSegmentEventMessage)({name:o.segmentEventType.onboardingStepCompleted,properties:{stepCompleted:er[n]}}).then(),ne.getState().finishOnboarding.action(!0)}),[n]);return(0,D.useEffect)((()=>{n||(0,o.setStore)("local",{onboardingStep:1}).then()}),[n]),n?(0,E.jsxs)(nr,{children:[(0,E.jsx)(un.v,{steps:r,currentStep:n}),1===n&&(0,E.jsx)(fn,{}),2===n&&(0,E.jsx)(xn,{}),3===n&&(0,E.jsx)(Qn,{}),4===n&&(0,E.jsx)(Xn,{}),4!==n&&(0,E.jsxs)(rr,{children:[(0,E.jsx)(je.R,{size:"100",buttonType:"tertiary",onClick:i,children:e("buttons.dismiss")}),(0,E.jsx)(je.R,{size:"200",onClick:a,children:e("buttons.next")})]})]}):(0,E.jsx)(or,{children:(0,E.jsx)(Se.xZ,{})})},nr=(0,B.default)(Se.Rm)`
  display: flex;
  flex-direction: column;
  flex-grow: 1;
  padding-top: ${({theme:e})=>e.spacing.sp16};
`,rr=B.default.div`
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: ${({theme:e})=>e.spacing.sp24};
`,or=B.default.div`
  width: 500px;
  min-height: 392px;
  align-items: center;
  flex-grow: 1;
  display: flex;
  justify-content: center;
`;var ar=Object.defineProperty,ir=Object.defineProperties,lr=Object.getOwnPropertyDescriptors,sr=Object.getOwnPropertySymbols,ur=Object.prototype.hasOwnProperty,cr=Object.prototype.propertyIsEnumerable,dr=(e,t,n)=>t in e?ar(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n,pr=(e,t)=>{for(var n in t||(t={}))ur.call(t,n)&&dr(e,n,t[n]);if(sr)for(var n of sr(t))cr.call(t,n)&&dr(e,n,t[n]);return e};const gr=e=>{const t=(0,Le.G)(),n=(0,D.useMemo)((()=>"seller"===t),[t]),{showSpinner:r,sync:i,cache:l}=(0,o.useShowSpinner)(),s=(0,D.useMemo)((()=>{var e;return!!(null==(e=null==i?void 0:i.onboardingFinished)?void 0:e[(null==i?void 0:i.accountId)||0])}),[i]),u=(0,D.useMemo)((()=>{var t,n;return"amazon"===e.platform&&!(null==(t=null==l?void 0:l.token)?void 0:t.hasMerchantIds)||"tiktok"===e.platform&&!(null==(n=null==l?void 0:l.token)?void 0:n.hasTiktokToken)}),[e.platform,l]),c=(0,D.useMemo)((()=>!!(null==i?void 0:i.isUserLogged)),[i]);return(0,E.jsx)(hr,{children:r?(0,E.jsx)(mr,{children:(0,E.jsx)(Se.xZ,{})}):c?(0,E.jsx)(E.Fragment,{children:s?(0,E.jsxs)(E.Fragment,{children:[(0,E.jsx)(nt,{isMainMenu:!1,isAmazon:(0,a.isAmazonPage)()}),(0,E.jsx)(rn,(d=pr({},e),p={isSellerMode:n},ir(d,lr(p)))),u&&n?(0,E.jsx)(fr,{children:(0,E.jsx)(o.TokenConnection,{isTiktok:"tiktok"===e.platform})}):void 0]}):(0,E.jsx)(tr,{})}):(0,E.jsxs)(E.Fragment,{children:[(0,E.jsx)(nt,{isMainMenu:!1,isAmazon:(0,a.isAmazonPage)()}),(0,E.jsx)(Re,{})]})});var d,p},hr=B.default.div`
  width: 100%;
  display: flex;
  flex-direction: column;
  position: relative;
`,mr=B.default.div`
  min-height: 439px;
  align-items: center;
  flex-grow: 1;
  display: flex;
  justify-content: center;
`,fr=B.default.div`
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: space-between;
  padding: ${({theme:e})=>`${e.spacing.sp16} 0`};
  border-top: 1px solid ${({theme:e})=>e.systemColors.grey200};
`;var vr=Object.defineProperty,yr=Object.defineProperties,kr=Object.getOwnPropertyDescriptors,xr=Object.getOwnPropertySymbols,wr=Object.prototype.hasOwnProperty,br=Object.prototype.propertyIsEnumerable,Cr=(e,t,n)=>t in e?vr(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n,Mr=(e,t)=>{for(var n in t||(t={}))wr.call(t,n)&&Cr(e,n,t[n]);if(xr)for(var n of xr(t))br.call(t,n)&&Cr(e,n,t[n]);return e};const Lr=e=>{const t=ne(me,G.X),[n,r]=(0,D.useState)(!1);return(0,D.useEffect)((()=>{t&&r(!0)}),[t]),(0,D.useEffect)((()=>{(0,o.sendBackgroundLoginMessage)({log:"WIDGET"}).then()}),[]),(0,E.jsx)(E.Fragment,{children:n?(0,E.jsx)(Sr,Mr({showPopup:t},e)):void 0})},Sr=e=>{var t=e,{showPopup:n}=t,r=((e,t)=>{var n={};for(var r in e)wr.call(e,r)&&t.indexOf(r)<0&&(n[r]=e[r]);if(null!=e&&xr)for(var r of xr(e))t.indexOf(r)<0&&br.call(e,r)&&(n[r]=e[r]);return n})(t,["showPopup"]);const{isInit:a,widgetPosition:i}=(0,o.useGetStore)("sync","widgetPosition"),[l,s]=(0,D.useState)(null),[u,c]=(0,D.useState)(0),d=(0,D.useMemo)((()=>-1*(i||0)),[i]),{mode:p}=(0,o.useGetStore)("sync","mode"),{entitlements:g}=(0,o.useGetCacheStore)("cache-sync",["entitlements"]);return(0,D.useEffect)((()=>{window.addEventListener("popstate",ue);const e=new ResizeObserver((()=>{c((null==l?void 0:l.clientHeight)||0)}));return l&&e.observe(l),()=>{window.removeEventListener("popstate",ue),e.disconnect()}}),[l]),a?(0,E.jsx)(E.Fragment,{}):(0,E.jsxs)(E.Fragment,{children:[(0,E.jsx)(jr,{$shown:n,onClick:ue}),(0,E.jsx)(Ir,{ref:s,$y:d,$height:u,$shown:n,onMouseEnter:le,onMouseLeave:se,children:(0,E.jsx)(o.ModalPlaceContext.Provider,{value:{current:l},children:(0,E.jsx)(Me.Z.Provider,{value:g,children:(0,E.jsx)(Le.$.Provider,{value:p||"seller",children:(0,E.jsx)(gr,(h=Mr({},r),m={showPopup:n},yr(h,kr(m))))})})})})]});var h,m},jr=B.default.div`
  display: ${({$shown:e})=>e?"block":"none"};
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  z-index: 1;
`,Ir=B.default.div`
  display: ${({$shown:e})=>e?"block":"none"};
  position: fixed;
  color: black;
  background-color: #fff;
  border: 1px solid #d9e0e8;
  box-shadow: 0 4px 24px -4px rgba(103, 132, 162, 0.4);
  border-radius: 16px;
  min-width: 400px;
  bottom: ${({$y:e,$height:t})=>Math.max(10,e+90-t-16)}px;
  right: 24px;
  z-index: 9999;
  padding: ${({theme:e})=>e.spacing.sp24};
  max-height: 800px;
  overflow-y: auto;
`;var Tr=Object.defineProperty,Pr=Object.getOwnPropertySymbols,Er=Object.prototype.hasOwnProperty,Dr=Object.prototype.propertyIsEnumerable,Or=(e,t,n)=>t in e?Tr(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n,Ar=(e,t)=>{for(var n in t||(t={}))Er.call(t,n)&&Or(e,n,t[n]);if(Pr)for(var n of Pr(t))Dr.call(t,n)&&Or(e,n,t[n]);return e};const Rr=e=>((0,D.useLayoutEffect)((()=>{e.initState&&ne.getState().showPopup.set(!0)}),[]),(0,E.jsxs)("div",{id:"h10-style-container",children:[(0,E.jsx)(ye,{}),(0,E.jsx)(Lr,Ar({},e))]}));var Nr=Object.defineProperty,Br=Object.getOwnPropertySymbols,$r=Object.prototype.hasOwnProperty,Gr=Object.prototype.propertyIsEnumerable,Fr=(e,t,n)=>t in e?Nr(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n,zr=(e,t)=>{for(var n in t||(t={}))$r.call(t,n)&&Fr(e,n,t[n]);if(Br)for(var n of Br(t))Gr.call(t,n)&&Fr(e,n,t[n]);return e};const Ur=e=>(0,E.jsx)(o.RootElement,{id:e.name,platform:e.platform,useShadow:!0,ignoreLogin:!0,fontsStyle:!0,faStyle:!0,isolatedGlobalStyle:!0,children:(0,E.jsx)(Rr,zr({},e))});const _r=(e,t,n)=>{return r=void 0,a=null,i=function*(){return(0,o.renderElement)((0,o.findCreateElement)("h10-page-widget"),(0,E.jsx)(Ur,{name:"h10-page-widget",platform:e,onAction:t,initState:n||!1}))},new Promise(((e,t)=>{var n=e=>{try{l(i.next(e))}catch(e){t(e)}},o=e=>{try{l(i.throw(e))}catch(e){t(e)}},l=t=>t.done?e(t.value):Promise.resolve(t.value).then(n,o);l((i=i.apply(r,a)).next())}));var r,a,i};const Wr=()=>{return e=void 0,t=null,n=function*(){return(0,o.renderElement)((0,o.findCreateElement)("toast-container"),(0,E.jsx)(o.ToastElement,{}))},new Promise(((r,o)=>{var a=e=>{try{l(n.next(e))}catch(e){o(e)}},i=e=>{try{l(n.throw(e))}catch(e){o(e)}},l=e=>e.done?r(e.value):Promise.resolve(e.value).then(a,i);l((n=n.apply(e,t)).next())}));var e,t,n};const Vr=e=>{return t=void 0,n=null,r=function*(){const t=(0,o.findCreateElement)("h10-upsell-modal");return t.style.position="relative",t.style.zIndex="1000001",(0,o.renderElement)(t,(0,E.jsx)(l.n,{platform:e}))},new Promise(((e,o)=>{var a=e=>{try{l(r.next(e))}catch(e){o(e)}},i=e=>{try{l(r.throw(e))}catch(e){o(e)}},l=t=>t.done?e(t.value):Promise.resolve(t.value).then(a,i);l((r=r.apply(t,n)).next())}));var t,n,r};var Hr=n(55963),Zr=n(62309),Kr=n(83001),Qr=n(73445),Yr=n(69897);function Xr(e,t="30"){return n=this,r=null,a=function*(){const n=yield(0,Yr._)(),r=new URL(`${o.membersLink}/api/v1/tiktok/product/sales`);n&&r.searchParams.set("accountId",String(n));const a=yield(0,V.H)({url:r.toString(),method:"POST",type:o.backgroundMessageTypes.BACKGROUND_REQUEST,params:{productId:e,lastFewDays:t},useSessionAccountId:!0});return{overviewChart:a.results.overviewChart,productInfo:a.results.productInfo}},new Promise(((e,t)=>{var o=e=>{try{l(a.next(e))}catch(e){t(e)}},i=e=>{try{l(a.throw(e))}catch(e){t(e)}},l=t=>t.done?e(t.value):Promise.resolve(t.value).then(o,i);l((a=a.apply(n,r)).next())}));var n,r,a}const qr=(0,Qr.Ue)(((e,t)=>({overviewData:null,productInfo:null,isLoading:!1,error:null,fetchedProductId:null,fetchOverviewData:n=>{return r=void 0,o=null,a=function*(){const{fetchedProductId:r,isLoading:o}=t();if(r!==n&&!o){e({isLoading:!0,error:null,overviewData:null,productInfo:null});try{const{overviewChart:t,productInfo:r}=yield Xr(n);e({overviewData:t,productInfo:r,isLoading:!1,fetchedProductId:n})}catch(t){const n=t instanceof Error?t.message:"Failed to fetch overview data";e({error:n,isLoading:!1})}}},new Promise(((e,t)=>{var n=e=>{try{l(a.next(e))}catch(e){t(e)}},i=e=>{try{l(a.throw(e))}catch(e){t(e)}},l=t=>t.done?e(t.value):Promise.resolve(t.value).then(n,i);l((a=a.apply(r,o)).next())}));var r,o,a}})));var Jr=n(76757),eo=n(62505),to=n(38562),no=n(56634);const ro=({review:e})=>(0,E.jsxs)(oo,{children:[(0,E.jsxs)(ao,{children:[(0,E.jsx)(io,{children:e.avatarUrl?(0,E.jsx)(lo,{src:e.avatarUrl,alt:e.userName}):(0,E.jsx)(so,{children:e.userName.charAt(0).toUpperCase()})}),(0,E.jsx)(uo,{children:(0,E.jsxs)(co,{children:[(0,E.jsxs)(po,{children:[(0,E.jsx)(go,{children:e.userName}),(0,E.jsx)(ho,{children:e.date})]}),(0,E.jsx)(eo.iG,{value:e.rating})]})})]}),(0,E.jsx)(no.u,{content:e.text,children:(0,E.jsx)(mo,{children:e.text})})]}),oo=B.default.div`
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 4px;
  background: #f8fafb;
  border-radius: 6px;
  padding: ${({theme:e})=>e.spacing.sp8};
  min-width: 0;
`,ao=B.default.div`
  display: flex;
  gap: ${({theme:e})=>e.spacing.sp8};
  align-items: flex-start;
  width: 100%;
`,io=B.default.div`
  width: 24px;
  height: 24px;
  border-radius: 50%;
  flex-shrink: 0;
  overflow: hidden;
  background: #d9e0e8;
  display: flex;
  align-items: center;
  justify-content: center;
`,lo=B.default.img`
  width: 100%;
  height: 100%;
  object-fit: cover;
`,so=B.default.span`
  font-family: "Nunito Sans", sans-serif;
  font-size: 12px;
  font-weight: 700;
  color: #485e75;
`,uo=B.default.div`
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  height: 24px;
  justify-content: center;
`,co=B.default.div`
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
`,po=B.default.div`
  display: flex;
  align-items: center;
  gap: 4px;
  white-space: nowrap;
`,go=B.default.span`
  ${Ie.nO};
  font-weight: 700;
  color: #13191f;
`,ho=B.default.span`
  ${Ie.Sv};
  color: #485e75;
`,mo=B.default.p`
  ${Ie.nO};
  color: #2b3948;
  margin: 0;
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
`;function fo(e){return e>=1e6?`${(e/1e6).toFixed(1).replace(/\.0$/,"")}M`:e>=1e3?`${(e/1e3).toFixed(1).replace(/\.0$/,"")}k`:e.toLocaleString()}function vo(e){return`$${Math.round(e).toLocaleString()}`}function yo(e){return e>=1e3?`$${(e/1e3).toFixed(1)}K`:`$${Math.round(e).toLocaleString()}`}const ko=({isLoading:e,isRestricted:t,overviewData:n,productInfo:r,pageData:a,onOpenTrendPanel:i,onOpenGmvTrendPanel:l,onAnalyzeReviews:s,onUpsell:u})=>{var c,d;const{t:p}=(0,o.useTranslation)();return(0,E.jsxs)(xo,{children:[(0,E.jsxs)(wo,{children:[(0,E.jsx)(bo,{children:p("tiktok.productToolbar.30DayItemsSold")}),e?(0,E.jsxs)(E.Fragment,{children:[(0,E.jsx)(Jr.h,{builder:[{size:"large",width:90,bottom:0}]}),(0,E.jsx)(Jr.h,{builder:[{size:"small",width:140,bottom:0}]})]}):(0,E.jsxs)(E.Fragment,{children:[(0,E.jsxs)(Co,{children:[(0,E.jsx)(Mo,{$blurred:t,onClick:t?u:void 0,children:t?fo(12400):n?fo(n.itemsSold):"-"}),!t&&n&&n.chartData.length>0&&(0,E.jsx)(Lo,{size:"50",buttonType:"icon",icon:(0,E.jsx)(A.G,{icon:O.Stf}),onClick:i})]}),(0,E.jsxs)(So,{$blurred:t,onClick:t?u:void 0,children:[p("tiktok.productToolbar.totalItemsSold")," ",t?fo(45875):null!=(c=null==a?void 0:a.totalItemsSold)?c:"N/A"]})]})]}),(0,E.jsxs)(wo,{children:[(0,E.jsx)(bo,{children:p("tiktok.productToolbar.30DayGmv")}),e?(0,E.jsxs)(E.Fragment,{children:[(0,E.jsx)(Jr.h,{builder:[{size:"large",width:110,bottom:0}]}),(0,E.jsx)(Jr.h,{builder:[{size:"small",width:140,bottom:0}]})]}):(0,E.jsxs)(E.Fragment,{children:[(0,E.jsxs)(Co,{children:[(0,E.jsx)(Mo,{$blurred:t,onClick:t?u:void 0,children:t?vo(45200):n?vo(n.gmv):"-"}),!t&&n&&n.chartData.length>0&&(0,E.jsx)(Lo,{size:"50",buttonType:"icon",icon:(0,E.jsx)(A.G,{icon:O.Stf}),onClick:l})]}),(0,E.jsxs)(So,{$blurred:t,onClick:t?u:void 0,children:[p("tiktok.productToolbar.totalGmv")," ",t?yo(189e3):r?yo(r.totalGmv):"-"]})]})]}),(0,E.jsxs)(jo,{children:[(0,E.jsxs)(Io,{children:[(0,E.jsx)(bo,{children:p("tiktok.productToolbar.currentRating")}),(0,E.jsxs)(To,{children:[(0,E.jsx)(eo.iG,{value:null!=(d=null==a?void 0:a.rating)?d:0,text:a?a.rating.toFixed(1):"-"}),(null==a?void 0:a.reviewCountText)&&(0,E.jsxs)(Po,{children:["(",a.reviewCountText,")"]})]}),(0,E.jsx)(Eo,{buttonType:"text",onClick:s,children:p("tiktok.productToolbar.analyzeReviews")})]}),(null==a?void 0:a.recentReview)&&(0,E.jsx)(ro,{review:a.recentReview})]})]})},xo=B.default.div`
  display: flex;
  gap: ${({theme:e})=>e.spacing.sp16};
  align-items: stretch;
  height: 112px;
`,wo=(0,B.default)(to.Q)`
  width: 220px;
  flex-shrink: 0;
  padding: ${({theme:e})=>e.spacing.sp16};
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: ${({theme:e})=>e.spacing.sp8};
`,bo=B.default.div`
  ${Ie.nO};
  color: #485e75;
  white-space: nowrap;
`,Co=B.default.div`
  display: flex;
  align-items: center;
  gap: ${({theme:e})=>e.spacing.sp8};
`,Mo=B.default.div`
  ${Ie.Xf};
  color: #13191f;
  ${({$blurred:e})=>e&&B.css`
      opacity: 0.5;
      filter: blur(4px);
      cursor: pointer;
      user-select: none;
    `}
`,Lo=(0,B.default)(je.R)`
  flex-shrink: 0;
  color: #485e75;
  padding: 4px;
  min-height: unset;
`,So=B.default.div`
  ${Ie.Sv};
  color: #485e75;
  ${({$blurred:e})=>e&&B.css`
      opacity: 0.5;
      filter: blur(4px);
      cursor: pointer;
      user-select: none;
    `}
`,jo=(0,B.default)(to.Q)`
  flex: 1;
  padding: ${({theme:e})=>e.spacing.sp16};
  display: flex;
  gap: ${({theme:e})=>e.spacing.sp16};
  align-items: center;
`,Io=B.default.div`
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
  flex-shrink: 0;
`,To=B.default.div`
  display: flex;
  align-items: center;
  gap: ${({theme:e})=>e.spacing.sp8};
`,Po=B.default.div`
  ${Ie.Sv};
  color: #485e75;
`,Eo=(0,B.default)(je.R)`
  padding: 0;
  min-height: unset;
  font-family: "Nunito Sans", sans-serif;
  font-weight: 700;
  font-size: 14px;
  line-height: 16px;
  color: #0a6fe8;
`;var Do=n(7543);const Oo=()=>{var e,t;const{t:n}=(0,o.useTranslation)(),r=document.querySelector("h1"),a=null!=(t=null==(e=null==r?void 0:r.innerText)?void 0:e.trim())?t:"",i="string"!=typeof(l=a)?"":l.length>40?`${l.slice(0,37)}...`:l;var l;return(0,E.jsx)(Ao,{children:(0,E.jsxs)(Ro,{children:[(0,E.jsx)(No,{children:(0,E.jsx)(Do.V,{width:19,height:20})}),(0,E.jsx)(Bo,{children:n("tiktok.productToolbar.productSummaryTitle")}),(0,E.jsx)(no.u,{content:a,children:(0,E.jsx)(Bo,{children:`"${i}"`})})]})})},Ao=B.default.div`
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
`,Ro=B.default.div`
  display: flex;
  align-items: center;
  gap: ${({theme:e})=>e.spacing.sp8};
`,No=B.default.div`
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
`,Bo=B.default.span`
  font-family: "Nunito Sans", sans-serif;
  font-weight: 800;
  font-size: 18px;
  line-height: 26px;
  color: #13191f;
  white-space: nowrap;
`;var $o=n(78840),Go=n.n($o),Fo=n(75708),zo=n.n(Fo),Uo=n(73935);function _o(e){const t=new Date(e);return`${t.getMonth()+1}/${t.getDate()}/${t.getFullYear()}`}const Wo=({visible:e,onClose:t,chartData:n,metric:r})=>{const{t:a}=(0,o.useTranslation)(),i=(0,o.useModalPlaceContext)(),[l,s]=(0,D.useState)(!1),u="gmv"===r;(0,D.useLayoutEffect)((()=>{"parent"!==i&&s(!0)}),[i]);const c=(0,D.useMemo)((()=>{const e=n.map((e=>[new Date(e.date).getTime(),u?e.gmv:e.itemsSold])),t=new Map(n.map((e=>[new Date(e.date).getTime(),_o(e.date)])));return{chart:{type:"line",backgroundColor:"transparent",height:400,margin:[20,24,48,52],style:{fontFamily:"'Nunito Sans', sans-serif"},animation:!1},title:{text:""},credits:{enabled:!1},legend:{enabled:!1},exporting:{enabled:!1},xAxis:{type:"datetime",tickInterval:1728e5,labels:{formatter(){const e=new Date(this.value);return`${e.getMonth()+1}/${e.getDate()}`},style:{fontSize:"12px",color:"#485e75",fontFamily:"'Nunito Sans', sans-serif"}},lineColor:"#d9e0e8",tickColor:"transparent",gridLineWidth:0,crosshair:{dashStyle:"Dot",color:"#a6b7c9",width:1,zIndex:1}},yAxis:{title:{text:""},gridLineColor:"#d9e0e8",gridLineWidth:1,labels:{style:{fontSize:"12px",color:"#485e75",fontFamily:"'Nunito Sans', sans-serif"}},min:0,tickAmount:5},tooltip:{useHTML:!0,backgroundColor:"transparent",borderWidth:0,shadow:!1,padding:0,outside:!1,formatter(){var e,n;const r=this.x,o=null!=(e=t.get(r))?e:"",i=null!=(n=this.y)?n:0;return`\n            <div class="tt-card">\n              <div class="tt-arrow tt-arrow-left">\n                <span class="tt-arrow-outer"></span>\n                <span class="tt-arrow-inner"></span>\n              </div>\n              <div class="tt-arrow tt-arrow-right" style="display:none">\n                <span class="tt-arrow-outer"></span>\n                <span class="tt-arrow-inner"></span>\n              </div>\n              <div class="tt-date">${o}</div>\n              <div class="tt-row">\n                <span class="tt-label">${a(u?"tiktok.productToolbar.salesTrend.gmv":"tiktok.productToolbar.salesTrend.itemsSold")}</span>\n                <strong class="tt-value">${u?`$${i.toLocaleString()}`:i}</strong>\n              </div>\n            </div>\n          `},positioner(e,t,n){const r=this.chart,o=n.plotX<.65*r.plotWidth,a=r.container,i=a.querySelector(".tt-arrow-left"),l=a.querySelector(".tt-arrow-right");i&&(i.style.display=o?"block":"none"),l&&(l.style.display=o?"none":"block");const s=o?r.plotLeft+n.plotX+12:r.plotLeft+n.plotX-e-12,u=r.plotTop+n.plotY-t/2;return{x:s,y:Math.max(r.plotTop,Math.min(u,r.plotTop+r.plotHeight-t))}}},plotOptions:{line:{color:"#138aea",lineWidth:2,states:{hover:{lineWidth:2,halo:{size:0}}},marker:{enabled:!1,symbol:"circle",states:{hover:{enabled:!0,radius:6,fillColor:"#004f94",lineWidth:4,lineColor:"#ffffff"}}}}},series:[{type:"line",name:"Items Sold",data:e}]}}),[n,u,a]);if(!e||!l)return null;if("parent"===i)return null;const d=(0,E.jsx)(Vo,{onClick:t,children:(0,E.jsxs)(Ho,{onClick:e=>e.stopPropagation(),children:[(0,E.jsx)(Zo,{children:a("tiktok.productToolbar.salesTrend.trends")}),(0,E.jsxs)(Ko,{children:[(0,E.jsxs)(Qo,{children:[(0,E.jsxs)(Yo,{children:[(0,E.jsx)(Xo,{children:a(u?"tiktok.productToolbar.salesTrend.gmvTitle":"tiktok.productToolbar.salesTrend.title")}),(0,E.jsx)(qo,{children:a(u?"tiktok.productToolbar.salesTrend.gmvSubtitle":"tiktok.productToolbar.salesTrend.subtitle")})]}),(0,E.jsx)(Jo,{onClick:t,"aria-label":"Close",children:(0,E.jsx)(A.G,{icon:O.g82})})]}),(0,E.jsx)(ea,{children:n.length>0?(0,E.jsx)(zo(),{highcharts:Go(),options:c}):(0,E.jsx)(ta,{children:a("tiktok.productToolbar.salesTrend.noData")})})]})]})});return(0,Uo.createPortal)(d,i)},Vo=B.default.div`
  position: fixed;
  inset: 0;
  z-index: 19999999997;
  background-color: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
`,Ho=B.default.div`
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: calc(100vh - 48px);
  overflow-y: auto;
`,Zo=B.default.div`
  ${Ie.nO};
  color: #485e75;
  padding-left: 4px;
`,Ko=B.default.div`
  background: #ffffff;
  border: 1px solid #d9e0e8;
  border-radius: 6px;
  width: 820px;
  box-shadow: 0 1px 8px rgba(103, 132, 162, 0.16), 0 2px 3px rgba(103, 132, 162, 0.08);
`,Qo=B.default.div`
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 24px 16px 16px 24px;
`,Yo=B.default.div`
  display: flex;
  flex-direction: column;
  gap: 4px;
  flex: 1;
  min-width: 0;
`,Xo=B.default.h3`
  ${Ie.f};
  color: #13191f;
  margin: 0;
`,qo=B.default.p`
  ${Ie.Qv};
  color: #485e75;
  margin: 0;
`,Jo=B.default.button`
  background: none;
  border: none;
  cursor: pointer;
  padding: 8px;
  color: #485e75;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 6px;
  flex-shrink: 0;
  font-size: 16px;
  line-height: 1;
  transition: background-color 0.15s;

  &:hover {
    background-color: #edf1f5;
  }
`,ea=B.default.div`
  padding: 0 24px 24px;

  .highcharts-container,
  .highcharts-root {
    width: 100% !important;
    /* Allow tooltip to overflow the chart bounds */
    overflow: visible !important;
  }

  /* Ensure tooltip layer sits above chart elements */
  .highcharts-tooltip {
    z-index: 100 !important;
    overflow: visible !important;
  }
  .highcharts-tooltip > span {
    overflow: visible !important;
  }

  /* Tooltip card */
  .tt-card {
    position: relative;
    background: #ffffff;
    border: 1px solid #a6b7c9;
    border-radius: 6px;
    padding: 14px 16px;
    box-shadow: 0 1px 8px rgba(103, 132, 162, 0.16), 0 2px 3px rgba(103, 132, 162, 0.08);
    font-family: "Nunito Sans", sans-serif;
    min-width: 160px;
    overflow: visible;
  }

  .tt-date {
    font-size: 14px;
    line-height: 18px;
    color: #485e75;
    margin-bottom: 8px;
  }

  .tt-row {
    font-size: 14px;
    line-height: 18px;
  }

  .tt-label {
    color: #138aea;
    font-weight: 400;
  }

  .tt-value {
    color: #13191f;
    font-weight: 700;
  }

  /* Shared arrow base — sized to match the outer triangle dimensions */
  .tt-arrow {
    position: absolute;
    top: 50%;
    width: 9px;
    height: 16px;
    transform: translateY(-50%);
    pointer-events: none;
  }

  .tt-arrow span {
    position: absolute;
    display: block;
    width: 0;
    height: 0;
  }

  /* Left arrow — tooltip is to the RIGHT of the data point */
  .tt-arrow-left {
    left: -9px;
  }

  .tt-arrow-left .tt-arrow-outer {
    top: 0;
    left: 0;
    border-top: 8px solid transparent;
    border-bottom: 8px solid transparent;
    border-right: 9px solid #a6b7c9;
  }

  /* Inner white triangle: 1px smaller on every side, shifted 1px inward */
  .tt-arrow-left .tt-arrow-inner {
    top: 1px;
    left: 1px;
    border-top: 7px solid transparent;
    border-bottom: 7px solid transparent;
    border-right: 8px solid #ffffff;
  }

  /* Right arrow — tooltip is to the LEFT of the data point */
  .tt-arrow-right {
    right: -9px;
  }

  .tt-arrow-right .tt-arrow-outer {
    top: 0;
    right: 0;
    border-top: 8px solid transparent;
    border-bottom: 8px solid transparent;
    border-left: 9px solid #a6b7c9;
  }

  .tt-arrow-right .tt-arrow-inner {
    top: 1px;
    right: 1px;
    border-top: 7px solid transparent;
    border-bottom: 7px solid transparent;
    border-left: 8px solid #ffffff;
  }
`,ta=B.default.div`
  ${Ie.nO};
  color: #6784a2;
  display: flex;
  align-items: center;
  justify-content: center;
  height: 300px;
`,na=()=>{var e;const t=document.querySelector("span.H3-Regular.text-color-UIText2:not(.ml-6)");if(!t)return;const n=null==(e=t.textContent)?void 0:e.trim().match(/([\d.]+[KkMm]?)/);return n?n[1]:void 0},ra=()=>{var e,t,n;const r=document.getElementById("pdp-review-section");if(!r)return null;const o=r.querySelector("div.flex.flex-col.gap-20.mb-20");if(!o)return null;const a=o.querySelector("div.H1-Bold.text-color-UIText1Display"),i=o.querySelector("div.H2-Semibold.text-color-UIText1Display");if(!a||!i)return null;const l=Number.parseFloat((null==(e=a.textContent)?void 0:e.trim())||"0"),s=null==(t=i.textContent)?void 0:t.match(/[\d,]+/),u=s?Number.parseInt(s[0].replace(/,/g,""),10):0;if(Number.isNaN(l)||0===l)return null;const c=(e=>{var t,n,r,o;const a=e.querySelectorAll(":scope > div.relative > div.flex.flex-col.gap-20 > div.flex.flex-col.gap-12")[0];if(!a)return null;const i=a.querySelector("div.H3-Semibold.text-color-UIText1"),l=(null==(t=null==i?void 0:i.textContent)?void 0:t.trim())||"",s=a.querySelector("div[role='img'][aria-label*='Rating']"),u=null==(n=null==s?void 0:s.getAttribute("aria-label"))?void 0:n.match(/(\d+)/),c=u?Number.parseInt(u[1],10):5,d=a.querySelector("div.H4-Regular.text-color-UIText1"),p=(null==(r=null==d?void 0:d.textContent)?void 0:r.trim())||"",g=a.querySelectorAll("div.H4-Regular.text-color-UIText3"),h=g[g.length-1],m=(null==(o=null==h?void 0:h.textContent)?void 0:o.trim())||"",f=a.querySelector("img.w-40.h-40"),v=(null==f?void 0:f.src)||"";return l&&p?{userName:l,date:m,rating:c,text:p,avatarUrl:v}:null})(r),d=(()=>{var e,t;const n=document.querySelector("span.H2-Semibold.mr-2.text-color-UIText1Display"),r=document.querySelector("span.ml-6.H3-Regular.text-color-UIText2");if(!n||!r)return;const o=Number.parseFloat((null==(e=n.textContent)?void 0:e.trim())||"0");if(Number.isNaN(o)||0===o)return;const a=(null==(t=r.textContent)?void 0:t.trim().replace(/^\(|\)$/g,""))||"";return a?{rating:o,reviewCountText:a}:void 0})();return{rating:null!=(n=null==d?void 0:d.rating)?n:l,reviewCount:u,recentReview:c,totalItemsSold:na(),reviewCountText:null==d?void 0:d.reviewCountText}},oa=()=>{const[e,t]=(0,D.useState)(null);return(0,D.useEffect)((()=>{let e=null,n=null;const r=()=>{const e=ra();return!!e&&(t(e),!0)},o=e=>{n=new MutationObserver((()=>{r()&&(null==n||n.disconnect())})),n.observe(e,{childList:!0,subtree:!0})};if(!r()){const t=document.getElementById("pdp-review-section");t?o(t):(e=new MutationObserver((()=>{const t=document.getElementById("pdp-review-section");t&&(null==e||e.disconnect(),e=null,r()||o(t))})),e.observe(document.body,{childList:!0,subtree:!0}))}return()=>{null==e||e.disconnect(),null==n||n.disconnect()}}),[]),e},aa=()=>{var e;const[t,n]=(0,D.useState)(null),{hasAccess:r}=(0,o.useEntitlements)("review-insight-with-tiktok"),a=!r,{t:i}=(0,o.useTranslation)(),{openToolSpecific:s}=(0,D.useMemo)((()=>(0,l.p)({globalTool:"TiktokReviewInsights",location:"TiktokProductToolbar"})),[]),{openToolSpecific:u}=(0,D.useMemo)((()=>(0,l.p)({globalTool:"TiktokProductToolbar",location:"TiktokProductToolbar"})),[]),{overviewData:c,productInfo:d,isLoading:p,fetchOverviewData:g}=qr(),h=oa(),m=(0,Kr.j)(),f=(0,D.useMemo)((()=>{const e=m.match(/\/(\d+)/);return(null==e?void 0:e[1])||""}),[m]);(0,D.useEffect)((()=>{f&&r&&g(f)}),[f,r,g]);const v=(0,D.useCallback)((()=>{var e;if(!r)return Zr.M.clear(),void s();null==(e=(0,o.getModalState)("h10-tiktok-review-insights"))||e.open({location:"tiktok-pdp-toolbar"},"open-tiktok-review-insights")}),[r,s]);return(0,E.jsxs)(ia,{children:[(0,E.jsx)(Oo,{}),(0,E.jsx)(ko,{isLoading:p,isRestricted:a,overviewData:c,productInfo:d,pageData:h,onOpenTrendPanel:()=>n("itemsSold"),onOpenGmvTrendPanel:()=>n("gmv"),onAnalyzeReviews:v,onUpsell:u}),(0,E.jsx)(la,{children:i("options.version",{version:chrome.runtime.getManifest().version_name||chrome.runtime.getManifest().version})}),(0,E.jsx)(Wo,{visible:null!==t,onClose:()=>n(null),chartData:null!=(e=null==c?void 0:c.chartData)?e:[],metric:null!=t?t:"itemsSold"})]})},ia=B.default.div`
  display: flex;
  flex-direction: column;
  gap: ${({theme:e})=>e.spacing.sp16};
  padding: ${({theme:e})=>e.spacing.sp16};
  font-family: "Nunito Sans", sans-serif;
  background: #ffffff;
  border: 1px solid #d9e0e8;
  border-radius: 6px;
`,la=B.default.div`
  ${Ie.Sv};
  color: #485e75;
  height: 16px;
  display: flex;
  align-items: center;
`,sa=()=>(0,E.jsx)(o.RootElement,{id:"tiktok-product-toolbar",platform:"tiktok",useShadow:!0,fontsStyle:!0,globalStyle:!0,faStyle:!0,children:(0,E.jsx)(aa,{})});const ua="h10-tiktok-product-toolbar",ca=()=>{const e=document.querySelector("ol[class*='list-none']");if(e)return e;const t=document.querySelectorAll("ol");for(const e of t)if(e.querySelector("li a"))return e;return null},da=()=>{return e=void 0,t=null,n=function*(){var e;if(!(0,a.isTiktokProductPage)())return Hr.ZT;if(!(yield new Promise((e=>{let t=0;const n=()=>{const r=ca();r?e(r):(t++,t>=30?e(null):setTimeout(n,1e3))};n()}))))return Hr.ZT;let t=null,n=Hr.ZT,r=null,i=null,l=0;const s=()=>{null!==i&&(clearTimeout(i),i=null)},u=()=>{n(),n=Hr.ZT,t=null},c=()=>{l>=10||(null!==r&&clearTimeout(r),r=setTimeout((()=>{if(r=null,document.getElementById(ua))return;const e=ca();if(!e)return;l++,u();const a=(e=>{const t=document.getElementById(ua);if(t)return t;const n=e.parentElement;if(!n)return null;const r=document.createElement("div");r.id=ua;const o=window.getComputedStyle(e),a=Number.parseFloat(o.paddingLeft),i=Number.parseFloat(o.paddingRight);return a>0&&(r.style.paddingLeft=`${a}px`),i>0&&(r.style.paddingRight=`${i}px`),r.style.paddingBottom="16px",n.insertBefore(r,e.nextSibling),r})(e);a&&(t=a,n=(0,o.renderElement)(a,(0,E.jsx)(sa,{})),s(),i=setTimeout((()=>{i=null,(null==t?void 0:t.isConnected)&&(l=0)}),2e3))}),300))},d=null!=(e=document.getElementById("root"))?e:document.body,p=new MutationObserver((()=>{document.getElementById(ua)||c()}));return p.observe(d,{childList:!0,subtree:!0}),c(),()=>{p.disconnect(),null!==r&&clearTimeout(r),s(),u()}},new Promise(((r,o)=>{var a=e=>{try{l(n.next(e))}catch(e){o(e)}},i=e=>{try{l(n.throw(e))}catch(e){o(e)}},l=e=>e.done?r(e.value):Promise.resolve(e.value).then(a,i);l((n=n.apply(e,t)).next())}));var e,t,n};const{updateElement:pa}=((e,t)=>{i[e]=t;const n={};return{updateElement:(t,r,o,a)=>{return l=void 0,s=null,u=function*(){var l,s,u;(null==(s=null==(l=i[e])?void 0:l[t])?void 0:s.call(l,o,a))?n[t]||(w&&console.log("ACTIVATE ELEMENT",t),n[t]=yield r()):n[t]&&(w&&console.log("DEACTIVATE ELEMENT",t),null==(u=n[t])||u.call(n),delete n[t])},new Promise(((e,t)=>{var n=e=>{try{o(u.next(e))}catch(e){t(e)}},r=e=>{try{o(u.throw(e))}catch(e){t(e)}},o=t=>t.done?e(t.value):Promise.resolve(t.value).then(n,r);o((u=u.apply(l,s)).next())}));var l,s,u},state:n}})("tiktok",{toast:()=>!0,pageWidget:(e,{pageWidget:t})=>!!t,productToolbar:(e,{listingHealthScore:t})=>e&&!!t,upsellModal:e=>e}),ga=(e,t,n)=>{return r=void 0,o=null,a=function*(){const n=(m||(chrome.runtime.onMessage.addListener(((e,t,n)=>v(e,n))),m=!0),(e,t)=>v({type:e,params:t}));yield pa("toast",(()=>Wr()),!0,t),yield pa("pageWidget",(()=>_r("tiktok",n)),!0,t),yield pa("upsellModal",(()=>Vr("tiktok")),e,t),yield pa("productToolbar",(()=>da()),e,t)},new Promise(((e,t)=>{var n=e=>{try{l(a.next(e))}catch(e){t(e)}},i=e=>{try{l(a.throw(e))}catch(e){t(e)}},l=t=>t.done?e(t.value):Promise.resolve(t.value).then(n,i);l((a=a.apply(r,o)).next())}));var r,o,a},ha={verifiedPurchase:!1,includeVisuals:!1,ratings:{1:!0,2:!0,3:!0,4:!0,5:!0},variations:{},dateRange:void 0,keywords:[]},ma=e=>e.replace(/[.*+?^${}()|[\]\\]/g,"\\$&"),fa=(e,t=[],n=[])=>{const r=[...e.keywords,...t].map((e=>{const t=e.trim().split(/\s+/).filter(Boolean).map((e=>ma(e)));if(0===t.length)return/(?!)/;const n=`(?:^|\\s)${t.join("\\s+")}\\b`;return new RegExp(n,"i")}));return{verifiedPurchase:e.verifiedPurchase,includeVisuals:e.includeVisuals,hasRatingFilter:Object.values(e.ratings).some((e=>!e)),ratings:e.ratings,hasVariationFilter:Object.values(e.variations).some((e=>!e)),variations:e.variations,variationGroups:n,dateRange:e.dateRange,keywordPatterns:r}},va=(e,t)=>{if(t.verifiedPurchase&&!e.isVerifiedPurchase)return!1;if(t.includeVisuals&&0===e.images.length)return!1;if(t.hasRatingFilter&&!t.ratings[e.rating])return!1;if(t.hasVariationFilter)if(t.variationGroups.length>0){const n=((e,t)=>{var n;const r=null!=(n=null==e?void 0:e.trim())?n:"";if(!r||0===t.length)return[];const o=r.toLowerCase().replace(/-/g," ");return t.map((({options:e})=>{const t=e.filter((e=>{const t=e.trim();if(0===t.length)return!1;const n=t.toLowerCase().replace(/-/g," ");return o.includes(n)})).sort(((e,t)=>t.length-e.length))[0];return t?t.trim():void 0}))})(e.variation,t.variationGroups);for(let e=0;e<t.variationGroups.length;e++){if(0===t.variationGroups[e].options.map((e=>e.trim())).filter(Boolean).filter((e=>!0===t.variations[e])).length)continue;const r=n[e];if(null==r||!t.variations[r])return!1}}else{const n=e.variation||"(Unknown)";if(n in t.variations&&!t.variations[n])return!1}if(t.keywordPatterns.length>0&&!t.keywordPatterns.every((t=>t.test(e.body))))return!1;if(t.dateRange){const[n,r]=t.dateRange,o=e.dateMs,a=new Date(n.getFullYear(),n.getMonth(),n.getDate()).getTime(),i=new Date(r.getFullYear(),r.getMonth(),r.getDate(),23,59,59,999).getTime();if(o<a||o>i)return!1}return!0};var ya=Object.defineProperty,ka=Object.defineProperties,xa=Object.getOwnPropertyDescriptors,wa=Object.getOwnPropertySymbols,ba=Object.prototype.hasOwnProperty,Ca=Object.prototype.propertyIsEnumerable,Ma=(e,t,n)=>t in e?ya(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n;const La={recommended:(e,t)=>t.helpfulCount-e.helpfulCount,recent:(e,t)=>new Date(t.date).getTime()-new Date(e.date).getTime()},Sa=e=>{const t=fa(e.filter,e.selectedPhrases,e.variationGroups);return e.reviews.filter((e=>va(e,t))).sort(La[e.sortBy])},ja=e=>{const{analysis:t,wordCount:n,reviews:r}=e,o=fa(e.filter,[],e.variationGroups),a=new Map(r.map((e=>[e.id,e])));return t.reduce(((e,t)=>{if(t.phrase.split(" ").length<n)return e;const r=t.reviewIds.filter((e=>{const t=a.get(e);return null!=t&&va(t,o)}));if(r.length>0){const n=r.reduce(((e,t)=>{var n,r;return e+(null!=(r=null==(n=a.get(t))?void 0:n.rating)?r:0)}),0);e.push(((e,t)=>ka(e,xa(t)))(((e,t)=>{for(var n in t||(t={}))ba.call(t,n)&&Ma(e,n,t[n]);if(wa)for(var n of wa(t))Ca.call(t,n)&&Ma(e,n,t[n]);return e})({},t),{reviewIds:r,count:r.length,rating:Math.round(n/r.length*10)/10}))}return e}),[])},Ia=e=>ja(e).slice(0,20),Ta=e=>{const t=Sa(e),n=new Map;for(const e of t){const t=e.variation||"",r=n.get(t);r?r.push(e):n.set(t,[e])}const r=t.length;return Array.from(n.entries()).map((([e,t])=>({variation:e,reviews:t,avgRating:Math.round(t.reduce(((e,t)=>e+t.rating),0)/t.length*10)/10,reviewShare:r>0?Math.round(t.length/r*100):0}))).sort(((e,t)=>t.reviews.length-e.reviews.length))};var Pa=Object.defineProperty,Ea=Object.defineProperties,Da=Object.getOwnPropertyDescriptors,Oa=Object.getOwnPropertySymbols,Aa=Object.prototype.hasOwnProperty,Ra=Object.prototype.propertyIsEnumerable,Na=(e,t,n)=>t in e?Pa(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n,Ba=(e,t)=>{for(var n in t||(t={}))Aa.call(t,n)&&Na(e,n,t[n]);if(Oa)for(var n of Oa(t))Ra.call(t,n)&&Na(e,n,t[n]);return e},$a=(e,t)=>Ea(e,Da(t));const Ga=(0,Qr.Ue)(((e,t)=>({chargeUseDailyLimitReached:!1,setChargeUseDailyLimitReached:t=>e({chargeUseDailyLimitReached:t}),view:"all",setView:t=>e({view:t}),reviews:[],setReviews:t=>{e({reviews:t,loadedCount:t.length,nextPageStart:2,showLoadMoreEndBanner:!1})},filter:Ba({},ha),setFilter:t=>e({filter:t}),patchFilter:t=>e((e=>({filter:Ba(Ba({},e.filter),t)}))),clearFilter:()=>{const n=t().filter.variations,r=Object.fromEntries(Object.keys(n).map((e=>[e,!0])));e({filter:$a(Ba({},ha),{variations:r}),selectedPhrases:[]})},resetForSyncRefresh:()=>{const n=t().filter.variations,r=Object.fromEntries(Object.keys(n).map((e=>[e,!0])));e({chargeUseDailyLimitReached:!1,view:"all",sortBy:"recommended",wordCount:2,filter:$a(Ba({},ha),{variations:r}),selectedPhrases:[],analysis:[],reviews:[],loadedCount:0,nextPageStart:2,isLoadingMore:!1,showLoadMoreEndBanner:!1})},selectedPhrases:[],togglePhrase:t=>e((e=>({selectedPhrases:e.selectedPhrases.includes(t)?e.selectedPhrases.filter((e=>e!==t)):[...e.selectedPhrases,t]}))),clearSelectedPhrases:()=>e({selectedPhrases:[]}),analysis:[],setAnalysis:t=>e({analysis:t}),sortBy:"recommended",setSortBy:t=>e({sortBy:t}),wordCount:2,setWordCount:t=>e({wordCount:t,selectedPhrases:[]}),isLoading:!0,setIsLoading:t=>e({isLoading:t}),isLoadingMore:!1,setIsLoadingMore:t=>e({isLoadingMore:t}),loadedCount:0,totalCount:null,setTotalCount:t=>e({totalCount:t}),hasMore:!1,setHasMore:t=>e({hasMore:t}),showLoadMoreEndBanner:!1,setShowLoadMoreEndBanner:t=>e({showLoadMoreEndBanner:t}),dismissLoadMoreEndBanner:()=>e({showLoadMoreEndBanner:!1}),nextPageStart:1,setNextPageStart:t=>e({nextPageStart:t}),appendReviews:t=>e((e=>({reviews:[...e.reviews,...t],loadedCount:e.reviews.length+t.length,nextPageStart:e.nextPageStart+1}))),variationGroups:[],productTitle:"",setProductTitle:t=>e({productTitle:t}),setVariationGroups:t=>{const n=t.flatMap((e=>e.options));e((e=>{var r;const o={};for(const t of n)o[t]=null==(r=e.filter.variations[t])||r;return{variationGroups:t,filter:$a(Ba({},e.filter),{variations:o})}}))}}))),Fa=()=>{const{t:e}=(0,o.useTranslation)();return Ga((e=>e.chargeUseDailyLimitReached))?(0,E.jsxs)(za,{children:[(0,E.jsx)(Ua,{icon:O.DBf}),(0,E.jsx)(_a,{children:e("tiktok.reviewInsights.chargeUse.dailyLimitMessage")})]}):null},za=B.default.div`
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 16px ${({theme:e})=>e.spacing.sp32};
  background-color: #fcf5d1;
  flex-shrink: 0;
`,Ua=(0,B.default)(A.G)`
  color: #a15c0e;
  font-size: 16px;
  flex-shrink: 0;
`,_a=B.default.span`
  font-size: 16px;
  color: #13191f;
  text-align: center;
`,Wa=({used:e,total:t,onUpgrade:n})=>{const{t:r}=(0,o.useTranslation)(),a=t>0?Math.min(e/t*100,100):0;return(0,E.jsxs)(Va,{children:[(0,E.jsx)(Ha,{children:(0,E.jsx)(Za,{style:{width:`${a}%`}})}),(0,E.jsxs)(Ka,{children:[(0,E.jsxs)(Qa,{children:[r("tiktok.reviewInsights.freeLaunchesUsed")," ",(0,E.jsxs)(Ya,{children:[e,"/",t]})]}),(0,E.jsx)(Xa,{onClick:n,children:r("tiktok.reviewInsights.upgradeToDiamond")})]})]})},Va=B.default.div`
  display: flex;
  align-items: center;
  gap: ${({theme:e})=>e.spacing.sp8};
  flex-shrink: 0;
`,Ha=B.default.div`
  width: 160px;
  height: 8px;
  background-color: ${({theme:e})=>e.systemColors.blue100};
  border-radius: ${({theme:e})=>{var t;return(null==(t=e.borderRadius)?void 0:t.small)||"4px"}};
  overflow: hidden;
  flex-shrink: 0;
`,Za=B.default.div`
  height: 100%;
  background-color: ${({theme:e})=>e.systemColors.blue600};
  border-radius: ${({theme:e})=>{var t;return(null==(t=e.borderRadius)?void 0:t.small)||"4px"}};
`,Ka=B.default.div`
  display: flex;
  align-items: center;
  gap: ${({theme:e})=>e.spacing.sp4};
  white-space: nowrap;
`,Qa=B.default.span`
  ${Ie.Qv};
  font-weight: 400;
  color: ${({theme:e})=>e.systemColors.grey600};
`,Ya=B.default.span`
  ${Ie.Qv};
  color: ${({theme:e})=>e.systemColors.grey600};
`,Xa=B.default.span`
  ${Ie.Qv};
  color: ${({theme:e})=>e.systemColors.blue600};
  cursor: pointer;

  &:hover {
    text-decoration: underline;
  }
`;var qa=n(64295),Ja=n(13493);const ei=e=>{(0,o.sendSegmentEventMessage)({name:o.segmentEventType.tiktokReviewInsightsEngagement,properties:{buttonName:e}})},ti=e=>{const[t,n,r]=e.split("-").map(Number);return new Date(t,n-1,r).getTime()},ni=e=>{var t,n,r;const o=(e=>{if(!e)return"";const t=String(e);if(/^\d{1,2}-\d{1,2}$/.test(t)){const e=(new Date).getFullYear(),[n,r]=t.split("-");return`${e}-${n.padStart(2,"0")}-${r.padStart(2,"0")}`}if(/^\d{10,}$/.test(t)){const e=Number.parseInt(t,10),n=10===t.length?1e3*e:e,r=new Date(n);return`${r.getFullYear()}-${String(r.getMonth()+1).padStart(2,"0")}-${String(r.getDate()).padStart(2,"0")}`}return t.replace(/\//g,"-")})(e.review_time);return{id:e.review_id,author:e.reviewer_name||"",country:e.review_country||"",date:o,dateMs:o?ti(o):0,variation:(e.sku_specification||"").trim(),rating:Math.min(5,Math.max(1,Number(e.review_rating)||5)),body:e.review_text||"",helpfulCount:0,images:(null==(t=e.review_images)?void 0:t.length)>0?e.review_images:[],avatarUrl:e.reviewer_avatar_url||"",variationImageUrl:e.display_image_url||"",skuId:e.sku_id||"",isVerifiedPurchase:null!=(n=e.is_verified_purchase)&&n,isRecommend:null!=(r=e.is_incentivized_review)&&r}},ri=e=>e.map(ni),oi=new Set(["a","am","an","and","are","as","b","be","but","c","d","do","don't","e","f","g","h","he","him","i","in","is","it","it's","k","l","m","my","n","o","of","on","or","p","q","r","s","she","t","the","they","there","this","u","v","w","was","were","where","with","without","who","whom","x","y","z","*","-"]),ai=new Set(["IS","TO","OF","AND","A","NO","THE"]),ii=new Set(["I","FOR"]),li=e=>{const t=new Map,n={};for(const r of e){const e=r.body.replace(/[\u2022,.!?:;]/g," ").replace(/\s+/g," ").trim().toLowerCase().split(" ").map((e=>oi.has(e)?"":e));for(let o=0;o<e.length;o++){if(!e[o])continue;let a="";for(let i=0;i<Math.min(9,e.length-o);i++){const l=e[o+i];if(!l)break;a=0===i?l:`${a} ${l}`;const s=t.get(a);s?(s.reviewIdSet.has(r.id)||(s.count++,s.reviewIdSet.add(r.id)),n[i]||(n[i]=[]),n[i].push(a)):t.set(a,{count:1,reviewIdSet:new Set([r.id]),phrase:a})}}}for(const[e,r]of Object.entries(n)){const o=n[Number.parseInt(e,10)-1];if(o)for(const e of o)if(r.some((t=>t.includes(e)))){const n=t.get(e);n&&n.count--}}const r=new Map(e.map((e=>[e.id,e])));return Array.from(t.values()).filter((e=>e.count>1&&!si(e.phrase))).map((e=>({phrase:e.phrase,count:e.count,reviewIds:Array.from(e.reviewIdSet),rating:ui(e.reviewIdSet,r)}))).sort(((e,t)=>t.count-e.count))},si=e=>{const t=e.toUpperCase(),n=t.indexOf(" "),r=t.lastIndexOf(" "),o=n>0?t.slice(0,n):t,a=r>0?t.slice(r+1):t;return ai.has(o)||ai.has(a)||ii.has(a)},ui=(e,t)=>{let n=0,r=0;for(const o of e){const e=t.get(o);e&&(n+=e.rating,r++)}return r>0?Math.round(n/r*10)/10:null};var ci=(e,t,n)=>new Promise(((r,o)=>{var a=e=>{try{l(n.next(e))}catch(e){o(e)}},i=e=>{try{l(n.throw(e))}catch(e){o(e)}},l=e=>e.done?r(e.value):Promise.resolve(e.value).then(a,i);l((n=n.apply(e,t)).next())}));const di={recommend:1,latest:2},pi={includesVisuals:2,verifiedPurchase:3},gi=()=>{var e;return null!=(e=window.location.pathname.split("/").pop())?e:""},hi=({pageStart:e=1,pageSize:t=500,sortRule:n="recommend",componentName:r="pdp_left_reviews",filterScore:o="all",filterSelectOption:a})=>({product_id:gi(),page_start:e,page_size:t,sort_rule:di[n],component_name:r,review_filter:{filter_value:"number"==typeof o?o:6,filter_type:a?pi[a]:1}}),mi=()=>ci(void 0,null,(function*(){const e=yield(0,Yr._)();if(!e||"-1"==`${e}`)return;const t=(0,o.getTiktokReviewChargeUseLink)(e);try{(e=>{const t=(e=>{if(!e||"object"!=typeof e)return null;const{results:t}=e;return t&&"object"==typeof t?t:null})(e),{setChargeUseDailyLimitReached:n}=Ga.getState();n(!!t&&(e=>"error"===e.status&&0===e.balance)(t))})(yield(0,V.H)({url:t,method:"POST",type:o.backgroundMessageTypes.BACKGROUND_REQUEST,params:{},useSessionAccountId:!0}))}catch(e){}}));class fi extends Error{constructor(){super("TiktokReviewChargeBlockedError"),this.name="TiktokReviewChargeBlockedError"}}const vi=e=>ci(void 0,null,(function*(){if(Ga.getState().chargeUseDailyLimitReached)throw new fi;if(yield mi(),Ga.getState().chargeUseDailyLimitReached)throw new fi;return(e=>(0,V.H)({url:`${window.location.origin}/api/shop/pdp_desktop/get_product_reviews`,type:o.backgroundMessageTypes.BACKGROUND_REQUEST,method:"POST",params:hi(e)}))(e)})),yi=(e,...t)=>ci(void 0,[e,...t],(function*(e,t=2){try{return yield vi(e)}catch(n){if(n instanceof fi)throw n;if(t<=0)throw n;return yi(e,t-1)}})),ki=e=>yi(e).then((e=>{var t,n,r,o,a;return{code:"number"==typeof(null==e?void 0:e.code)?e.code:-1,has_more:null!=(n=null==(t=null==e?void 0:e.data)?void 0:t.has_more)&&n,product_reviews:null!=(o=null==(r=null==e?void 0:e.data)?void 0:r.product_reviews)?o:[],review_ratings:null==(a=null==e?void 0:e.data)?void 0:a.review_ratings}})),xi=e=>"recent"===e?"latest":"recommend",wi=B.keyframes`
  to {
    transform: rotate(360deg);
  }
`,bi=B.keyframes`
  0%,
  100% {
    opacity: 0.4;
  }
  50% {
    opacity: 1;
  }
`,Ci=B.default.span`
  display: inline-block;
  flex-shrink: 0;
  width: 1em;
  height: 1em;
  border-radius: 50%;
  box-sizing: border-box;
  border: 0.15em solid currentColor;
  border-right-color: transparent;
  border-bottom-color: transparent;
  transform-origin: center center;
  animation:
    ${wi} 0.75s linear infinite,
    ${bi} 1s ease-in-out infinite;
`,Mi=()=>{const{t:e}=(0,o.useTranslation)(),{loadMore:t,hasMore:n,isLoadingMore:r}=(()=>{const e=Ga((e=>e.appendReviews)),t=Ga((e=>e.setAnalysis)),n=Ga((e=>e.setHasMore)),r=Ga((e=>e.setShowLoadMoreEndBanner)),o=Ga((e=>e.setIsLoadingMore)),a=Ga((e=>e.sortBy)),i=Ga((e=>e.hasMore)),l=Ga((e=>e.isLoading)),s=Ga((e=>e.isLoadingMore));return{loadMore:(0,D.useCallback)((()=>{if(!i||l||s)return;const{nextPageStart:u}=Ga.getState();o(!0),ki({pageStart:u,pageSize:500,sortRule:xi(a)}).then((({code:o,has_more:a,product_reviews:i})=>{if(0===o&&0===i.length)return n(!1),void r(!0);const l=ri(i);e(l);const s=Ga.getState().reviews;t(li(s)),n(a)})).catch((()=>{n(!1)})).finally((()=>{o(!1)}))}),[i,l,s,a,e,t,n,r,o]),hasMore:i,isLoadingMore:s}})(),a=Ga((e=>e.isLoading)),i=Ga((e=>e.showLoadMoreEndBanner)),l=Ga((e=>e.dismissLoadMoreEndBanner));return i?(0,E.jsxs)(Ii,{children:[(0,E.jsx)(Ti,{"aria-hidden":!0,children:(0,E.jsx)(A.G,{icon:O.sqG})}),(0,E.jsx)(Pi,{children:e("tiktok.reviewInsights.reviews.loadMoreAllRetrieved")}),(0,E.jsx)(Ei,{onClick:l,children:(0,E.jsx)(A.G,{icon:O.YIN})})]}):n?(0,E.jsx)(Li,{children:r?(0,E.jsxs)(Si,{"aria-live":"polite",children:[(0,E.jsx)(Ci,{"aria-hidden":!0}),(0,E.jsx)(ji,{children:e("tiktok.reviewInsights.reviews.loadMore")})]}):(0,E.jsx)(je.R,{buttonType:"text",disabled:a,onClick:()=>{ei("loadMore"),t()},children:e("tiktok.reviewInsights.reviews.loadMore")})}):null},Li=B.default.div`
  display: flex;
  height: 50px;
  align-items: center;
  padding: ${({theme:e})=>e.spacing.sp16};
  flex-shrink: 0;
  border-top: 1px solid ${({theme:e})=>e.systemColors.grey200};
`,Si=B.default.output`
  display: inline-flex;
  align-items: center;
  gap: ${({theme:e})=>e.spacing.sp8};
  margin: 0;
  padding: 0;
  border: none;
  color: ${({theme:e})=>e.systemColors.grey800};
  font-size: 14px;
`,ji=B.default.span`
  ${Ie.nO};
`,Ii=B.default.div`
  display: flex;
  height: 50px;
  align-items: center;
  gap: ${({theme:e})=>e.spacing.sp12};
  padding: ${({theme:e})=>e.spacing.sp12} ${({theme:e})=>e.spacing.sp16};
  border-radius: 4px;
  background: ${"#E8F6FF"};
  position: relative;
`,Ti=B.default.span`
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
  color: ${"#0253B6"};
  font-size: 18px;
  line-height: 1;
`,Pi=B.default.p`
  margin: 0;
  flex: 1;
  ${Ie.nO};
  color: #13191f;
  line-height: 1.4;
`,Ei=B.default.span`
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
  color: #485e75;
  font-size: 18px;
  line-height: 1;
  cursor: pointer;
`,Di=({used:e=10,total:t=10,onUpgrade:n})=>{const{t:r}=(0,o.useTranslation)(),a=t>0?Math.min(e/t*100,100):0,i=e>=t;return(0,E.jsxs)(Oi,{children:[(0,E.jsxs)(Ai,{children:[(0,E.jsx)(Ri,{children:(0,E.jsx)(Ni,{$exhausted:i,style:{width:`${a}%`}})}),(0,E.jsxs)(Bi,{children:[r("tiktok.reviewInsights.freeLaunchesUsed")," ",(0,E.jsxs)($i,{children:[e,"/",t]})]})]}),(0,E.jsxs)(Gi,{children:[(0,E.jsx)(Fi,{children:r("tiktok.reviewInsights.reachedLimit.title")}),(0,E.jsx)(zi,{children:r("tiktok.reviewInsights.reachedLimit.subtitle")})]}),(0,E.jsx)(Ui,{buttonType:"primary",onClick:n,children:r("tiktok.reviewInsights.upgradeToDiamond")})]})},Oi=B.default.div`
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: ${({theme:e})=>e.spacing.sp16};
  padding: ${({theme:e})=>e.spacing.sp48} ${({theme:e})=>e.spacing.sp24};
  width: 100%;
`,Ai=B.default.div`
  display: flex;
  align-items: center;
  gap: ${({theme:e})=>e.spacing.sp8};
  width: 320px;
`,Ri=B.default.div`
  flex: 1 0 0;
  min-width: 100px;
  height: 8px;
  background-color: ${({theme:e})=>e.systemColors.blue100};
  border-radius: ${({theme:e})=>{var t;return(null==(t=e.borderRadius)?void 0:t.small)||"4px"}};
  overflow: hidden;
`,Ni=B.default.div`
  height: 100%;
  border-radius: ${({theme:e})=>{var t;return(null==(t=e.borderRadius)?void 0:t.small)||"4px"}};
  background-color: ${({theme:e,$exhausted:t})=>t?e.dataVisualizationColors.tomato500:e.systemColors.blue600};
`,Bi=B.default.span`
  ${Ie.Qv};
  font-weight: 400;
  color: ${({theme:e})=>e.systemColors.grey600};
  white-space: nowrap;
  flex-shrink: 0;
`,$i=B.default.span`
  ${Ie.Qv};
  color: ${({theme:e})=>e.systemColors.grey600};
`,Gi=B.default.div`
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: ${({theme:e})=>e.spacing.sp8};
  width: 100%;
`,Fi=B.default.p`
  ${Ie.uh};
  color: ${({theme:e})=>e.systemColors.black};
  text-align: center;
  margin: 0;
`,zi=B.default.p`
  ${Ie.Qv};
  font-weight: 400;
  color: ${({theme:e})=>e.systemColors.grey600};
  text-align: center;
  margin: 0;
`,Ui=(0,B.default)(je.R)`
  background-color: ${({theme:e})=>e.systemColors.blue600};
  color: ${({theme:e})=>e.systemColors.white};
  padding: ${({theme:e})=>e.spacing.sp12} ${({theme:e})=>e.spacing.sp16};
  border-radius: ${({theme:e})=>{var t;return(null==(t=e.borderRadius)?void 0:t.sp6)||"6px"}};
  font-size: 16px;
  font-weight: 700;
  line-height: 16px;
  white-space: nowrap;
`;var _i=n(86382),Wi=n(15694);const Vi=B.default.div`
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 100%;
  padding: ${({theme:e})=>e.spacing.sp48} ${({theme:e})=>e.spacing.sp32};
  text-align: center;
  background: ${({theme:e})=>e.systemColors.white};
  border-top: 1px solid ${({theme:e})=>e.systemColors.grey220};
  ${Ie.nn};
  font-size: 24px;
  font-weight: 800;
  line-height: 1.25;
  color: ${({theme:e})=>e.systemColors.black};
`,Hi=B.default.div`
  height: auto;
  overflow: hidden;
  ${({$borderTop:e,theme:t})=>e?`border-top: solid 1px ${t.systemColors.grey200};`:""}
  animation: slide-down 0.4s ease;

  @keyframes slide-down {
    0% {
      opacity: 0;
      transform: translateY(-20px);
    }
    100% {
      opacity: 1;
      transform: translateY(0);
    }
  }
`,Zi=(0,B.default)(Wi.gG)`
  padding: 0;
  border: none;

  button {
    justify-content: unset !important;
    color: ${({theme:e})=>e.systemColors.grey600};
    margin-left: ${({theme:e})=>e.spacing.sp8};
    min-height: 16px;
    padding: 0;

    :hover {
      color: ${({theme:e})=>e.systemColors.grey600};
    }

    :focus {
      color: ${({theme:e})=>e.systemColors.grey600};
    }
  }
`,Ki=B.default.div`
  ${Ie.nn};
  color: ${({theme:e})=>e.systemColors.black};
`,Qi=()=>{const{t:e}=(0,o.useTranslation)(),t=Ga((e=>e.clearFilter));return(0,E.jsxs)(Yi,{children:[(0,E.jsx)(Xi,{children:e("tiktok.reviewInsights.reviews.noReviews")}),(0,E.jsx)(qi,{children:e("tiktok.reviewInsights.reviews.noReviewsFilterDescription")}),(0,E.jsx)(Ji,{children:(0,E.jsx)(je.R,{size:"200",buttonType:"secondary",onClick:t,children:e("tiktok.reviewInsights.reviews.clearFilters")})})]})},Yi=B.default.div`
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: ${({theme:e})=>e.spacing.sp12};
  padding: ${({theme:e})=>e.spacing.sp48} ${({theme:e})=>e.spacing.sp32};
  min-height: 100%;
  text-align: center;
  background: ${({theme:e})=>e.systemColors.white};
  border-top: 1px solid ${({theme:e})=>e.systemColors.grey220};
`,Xi=B.default.div`
  ${Ie.nn};
  font-weight: 800;
  font-size: 24px;
  line-height: 1.25;
  color: ${({theme:e})=>e.systemColors.black};
`,qi=B.default.div`
  ${Ie.nO};
  font-size: 16px;
  line-height: 1.5;
  color: ${({theme:e})=>e.systemColors.grey600};
  max-width: 600px;
`,Ji=B.default.div`
  margin-top: ${({theme:e})=>e.spacing.sp8};
`,el=()=>{const{t:e}=(0,o.useTranslation)(),[t,n]=(0,D.useState)(!1),r=Ga(Ia),a=Ga((e=>e.selectedPhrases)),i=Ga((e=>e.togglePhrase)),l=Ga((e=>e.reviews.length)),s=Ga((e=>e.isLoading))||0===l;return(0,E.jsx)(tl,{title:(0,E.jsx)(Ki,{children:e("tiktok.reviewInsights.phrases.title")}),onOpenChange:e=>n(e),$disabled:s,buttonText:e(t?"tiktok.reviewInsights.phrases.hidePhrases":"tiktok.reviewInsights.phrases.showPhrases"),children:(0,E.jsx)(nl,{children:(0,E.jsx)(rl,{children:r.map((({phrase:e,count:t})=>(0,E.jsx)(_i.G,{active:a.includes(e),size:"50",content:`${e} (${t})`,onClick:()=>{ei("topPhrasesSelected"),i(e)}},e)))})})})},tl=(0,B.default)(Wi.gG)`
  ${Ja.pb};
  padding: ${({theme:e})=>e.spacing.sp16} !important;
  
  button {
    ${({$disabled:e,theme:t})=>e&&`color: ${t.systemColors.grey300}; pointer-events: none;`}
  }

`,nl=B.default.div`
  margin-top: ${({theme:e})=>e.spacing.sp16};
`,rl=B.default.div`
  display: flex;
  flex-wrap: wrap;
  overflow: hidden;
  gap: ${({theme:e})=>e.spacing.sp8};
`;var ol=n(42283),al=n(74239);const il=()=>{const{t:e}=(0,o.useTranslation)();return(0,E.jsxs)(sl,{children:[(0,E.jsx)(ll,{name:"verifiedPurchase",label:e("tiktok.reviewInsights.filters.verifiedPurchase")}),(0,E.jsx)(ll,{name:"includeVisuals",label:e("tiktok.reviewInsights.filters.includeVisuals")})]})},ll=({name:e,label:t})=>{const{control:n}=(0,ol.Gc)();return(0,E.jsx)(ol.Qr,{name:e,control:n,render:({value:e,onChange:n})=>(0,E.jsx)(al.q,{checked:e,onChange:({target:e})=>n(e.checked),children:t})})},sl=B.default.div`
  display: flex;
  flex-direction: column;
  gap: ${({theme:e})=>e.spacing.sp4};
`;var ul=n(56486);const cl=()=>{const{t:e}=(0,o.useTranslation)(),{control:t}=(0,ol.Gc)();return(0,E.jsx)(ol.Qr,{name:"dateRange",control:t,render:({value:t,onChange:n})=>(0,E.jsx)(dl,{size:"50",value:t,placeholder:e("tiktok.reviewInsights.filters.selectDateRange"),onChange:n,onClear:()=>n(void 0),mode:"range",columns:1,showCalendar:!0,disabledInput:!0,maxDate:new Date,appendTo:"parent"})})},dl=(0,B.default)(ul.W)`
  height: 24px;
`;var pl=n(33657);const gl=()=>{const{t:e}=(0,o.useTranslation)(),{control:t}=(0,ol.Gc)(),n=Ga((e=>e.selectedPhrases)),[r,a]=(0,D.useState)([]),i=(0,D.useCallback)(((e,t)=>{const n=e.map((e=>e.toLowerCase()));a(n),t(n)}),[]);return(0,D.useEffect)((()=>a(n)),[n]),(0,E.jsx)(ol.Qr,{name:"keywords",control:t,render:({onChange:t})=>(0,E.jsx)(pl.c,{value:r,onChange:e=>i(e,t),inputProps:{placeholder:e("tiktok.reviewInsights.filters.enterKeyword")}})})};var hl=Object.defineProperty,ml=Object.defineProperties,fl=Object.getOwnPropertyDescriptors,vl=Object.getOwnPropertySymbols,yl=Object.prototype.hasOwnProperty,kl=Object.prototype.propertyIsEnumerable,xl=(e,t,n)=>t in e?hl(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n,wl=(e,t)=>{for(var n in t||(t={}))yl.call(t,n)&&xl(e,n,t[n]);if(vl)for(var n of vl(t))kl.call(t,n)&&xl(e,n,t[n]);return e};const bl=[5,4,3,2,1],Cl={1:!1,2:!1,3:!1,4:!1,5:!1},Ml=()=>{const{t:e}=(0,o.useTranslation)(),{control:t}=(0,ol.Gc)();return(0,E.jsx)(ol.Qr,{name:"ratings",control:t,rules:{validate:e=>!Object.values(e).every((e=>!e))},render:({value:t,onChange:n})=>(0,E.jsx)(Zi,{buttonText:" ",title:(0,E.jsx)(al.q,{checked:Object.values(t).every(Boolean),onChange:Ll(n),children:e("tiktok.reviewInsights.filters.allRatings")}),children:(0,E.jsx)(jl,{children:bl.map((r=>{var o;return(0,E.jsx)(al.q,{checked:null==(o=t[r])||o,onChange:()=>Sl(r,t,n),children:e("tiktok.reviewInsights.stars",{count:r})},r)}))})})})},Ll=e=>t=>{e(t.target.checked?wl({},ha.ratings):wl({},Cl))},Sl=(e,t,n)=>{const r=(o=wl({},t),a={[e]:!t[e]},ml(o,fl(a)));var o,a;n(Object.values(r).every(Boolean)?wl({},ha.ratings):r)},jl=B.default.div`
  display: flex;
  flex-direction: column;
  gap: ${({theme:e})=>e.spacing.sp4};
  margin-top: 6px;
`;var Il=Object.defineProperty,Tl=Object.defineProperties,Pl=Object.getOwnPropertyDescriptors,El=Object.getOwnPropertySymbols,Dl=Object.prototype.hasOwnProperty,Ol=Object.prototype.propertyIsEnumerable,Al=(e,t,n)=>t in e?Il(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n;const Rl=()=>{const{t:e}=(0,o.useTranslation)(),{control:t}=(0,ol.Gc)(),n=Ga((e=>e.variationGroups));return 0===n.length?null:(0,E.jsx)(ol.Qr,{control:t,name:"variations",rules:{validate:e=>0===Object.keys(null!=e?e:{}).length||Object.values(e).some(Boolean)},render:({value:t,onChange:r})=>(0,E.jsx)(Zi,{buttonText:" ",title:(0,E.jsx)(al.q,{checked:Object.values(t).every((e=>e)),onChange:Nl(t,r),children:e("tiktok.reviewInsights.filters.allVariations")}),children:(0,E.jsx)($l,{children:n.map(((e,n)=>(0,E.jsxs)(Gl,{children:[e.label&&(0,E.jsxs)(Fl,{children:[e.label,":"]}),(0,E.jsx)(zl,{children:e.options.map((e=>{var n;return(0,E.jsx)(no.u,{colorType:"light",content:e,children:(0,E.jsx)(Ul,{active:null==(n=t[e])||n,size:"50",content:e,onClick:()=>Bl(e,t,r)})},e)}))})]},`${e.label||"group"}-${n}`)))})})})},Nl=(e,t)=>n=>{t(Object.fromEntries(Object.keys(e).map((e=>[e,n.target.checked]))))},Bl=(e,t,n)=>{var r,o;n((r=((e,t)=>{for(var n in t||(t={}))Dl.call(t,n)&&Al(e,n,t[n]);if(El)for(var n of El(t))Ol.call(t,n)&&Al(e,n,t[n]);return e})({},t),o={[e]:!t[e]},Tl(r,Pl(o))))},$l=B.default.div`
  display: flex;
  flex-direction: column;
  gap: ${({theme:e})=>e.spacing.sp8};
  margin-top: 6px;
`,Gl=B.default.div`
  display: flex;
  flex-direction: column;
  gap: ${({theme:e})=>e.spacing.sp4};
`,Fl=B.default.span`
  ${Ie.nn};
  color: ${({theme:e})=>e.systemColors.grey600};
  font-size: 12px;
`,zl=B.default.div`
  display: flex;
  flex-wrap: wrap;
  gap: ${({theme:e})=>e.spacing.sp4};
`,Ul=(0,B.default)(_i.G)`
  span {
    max-width: 100px;
    text-overflow: ellipsis;
    overflow: hidden;
  }
`;var _l=Object.defineProperty,Wl=Object.defineProperties,Vl=Object.getOwnPropertyDescriptors,Hl=Object.getOwnPropertySymbols,Zl=Object.prototype.hasOwnProperty,Kl=Object.prototype.propertyIsEnumerable,Ql=(e,t,n)=>t in e?_l(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n,Yl=(e,t)=>{for(var n in t||(t={}))Zl.call(t,n)&&Ql(e,n,t[n]);if(Hl)for(var n of Hl(t))Kl.call(t,n)&&Ql(e,n,t[n]);return e};const Xl=()=>{const{t:e}=(0,o.useTranslation)(),[t,n]=(0,D.useState)(!1),r=Ga((e=>e.filter)),a=Ga((e=>e.patchFilter)),i=Ga((e=>e.clearFilter)),l=Ga((e=>e.reviews.length)),s=Ga((e=>e.isLoading))||0===l,u=(0,D.useMemo)((()=>Yl({},r)),[r]),c=(0,ol.cI)({defaultValues:u,mode:"onChange"}),{handleSubmit:d,reset:p,control:g}=c,h=((e,t)=>{var n={};for(var r in e)Zl.call(e,r)&&t.indexOf(r)<0&&(n[r]=e[r]);if(null!=e&&Hl)for(var r of Hl(e))t.indexOf(r)<0&&Kl.call(e,r)&&(n[r]=e[r]);return n})(c,["handleSubmit","reset","control"]);(0,D.useEffect)((()=>p(u)),[u,p]);const m=(0,D.useCallback)((()=>{i(),g.setValue("dateRange",void 0)}),[i,g]),f=h.watch(),v=(0,D.useMemo)((()=>{var e,t;const n=Object.values(null!=(e=f.ratings)?e:ha.ratings).some(Boolean),r=Object.values(null!=(t=f.variations)?t:{}),o=0===r.length||r.some(Boolean);return s||!n||!o}),[s,f]);return(0,E.jsx)(ql,{title:(0,E.jsx)(Ki,{children:e("tiktok.reviewInsights.filters.title")}),onOpenChange:e=>n(e),buttonText:e(t?"tiktok.reviewInsights.filters.hideFilters":"tiktok.reviewInsights.filters.showFilters"),$disabled:s,children:(0,E.jsx)(ol.RV,(y=Yl({handleSubmit:d,reset:p,control:g},h),k={children:(0,E.jsxs)(Jl,{onSubmit:d((e=>{const t=e.verifiedPurchase||e.includeVisuals||Object.values(e.ratings).some((e=>!e))||void 0!==e.dateRange||e.keywords.length>0,n=Object.values(e.variations).some((e=>!e));t&&ei("filtersSelected"),n&&ei("variationSelected"),a(e)})),children:[(0,E.jsx)(cl,{}),(0,E.jsx)(il,{}),(0,E.jsx)(Ml,{}),(0,E.jsx)(Rl,{}),(0,E.jsx)(gl,{}),(0,E.jsxs)(es,{children:[(0,E.jsx)(je.R,{type:"button",onClick:m,buttonType:"secondary",disabled:s,children:e("tiktok.reviewInsights.filters.clear")}),(0,E.jsx)(je.R,{type:"submit",disabled:v,children:e("tiktok.reviewInsights.filters.applyFilters")})]})]})},Wl(y,Vl(k))))});var y,k},ql=(0,B.default)(Wi.gG)`
  ${Ja.pb};
  padding: ${({theme:e})=>e.spacing.sp16} !important;

  button {
    ${({$disabled:e,theme:t})=>e&&`color: ${t.systemColors.grey300}; pointer-events: none;`}
  }
`,Jl=B.default.form`
  display: flex;
  flex-direction: column;
  gap: ${({theme:e})=>e.spacing.sp24};
  margin-top: ${({theme:e})=>e.spacing.sp16};
`,es=B.default.div`
  display: flex;
  gap: ${({theme:e})=>e.spacing.sp8};
`,ts=B.keyframes`
  0% { width: 0%; }
  5% { width: 20%; }
  65% { width: 25%; }
  80% { width: 80%; }
  100% { width: 90%; }
`,ns=B.keyframes`
  from { width: 90%; }
  to { width: 100%; }
`,rs=()=>{const{t:e}=(0,o.useTranslation)(),{status:t}=(0,qa.A)();return(0,E.jsxs)(os,{children:[(0,E.jsx)(as,{children:e("reviewInsight.loading.loadingTitle")}),(0,E.jsx)(is,{children:e("reviewInsight.loading.loadingSubtitle")}),(0,E.jsx)(ls,{children:(0,E.jsx)(ss,{$status:t})})]})},os=B.default.div`
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  height: 100%;
  gap: ${({theme:e})=>e.spacing.sp16};
  text-align: center;
`,as=B.default.div`
  ${Ie.f};
`,is=B.default.div`
  ${Ie.Qv};
  margin-bottom: ${({theme:e})=>e.spacing.sp16};
`,ls=B.default.div`
  width: 250px;
  height: 8px;
  background-color: ${({theme:e})=>{var t,n,r;return null!=(r=null!=(n=e.systemColors.grey200)?n:null==(t=e.colors)?void 0:t.lightGrey)?r:"#e4e9ec"}};
  border-radius: 4px;
  overflow: hidden;
`,ss=B.default.div`
  height: 100%;
  width: 0;
  border-radius: 4px;
  background-color: ${({theme:e})=>{var t,n,r;return null!=(r=null!=(n=e.systemColors.blue600)?n:null==(t=e.colors)?void 0:t.blue)?r:"#1890ff"}};

  ${({$status:e})=>"start"===e&&B.css`
      animation: ${ts} 5s ease-out forwards;
    `}
  ${({$status:e})=>"finish"===e&&B.css`
      animation: ${ns} 0.25s ease-out forwards;
    `}
`;var us=n(51398);const cs=["all","keywords","variations"],ds=()=>{const{t:e}=(0,o.useTranslation)(),t=Ga((e=>e.view)),n=Ga((e=>e.setView)),r=Ga((e=>e.reviews.length)),a=Ga((e=>e.isLoading))||0===r,i=(0,D.useMemo)((()=>cs.map((t=>({children:e(`tiktok.reviewInsights.views.${t}.tab`),value:t})))),[e]);return(0,E.jsxs)(to.Q,{padding:"small",children:[(0,E.jsx)(ps,{children:e("tiktok.reviewInsights.tabSwitcher.title")}),(0,E.jsx)(us.Z,{size:"50",value:t,buttons:i,disabled:a,onChange:n})]})},ps=B.default.h3`
  ${Ie.nn};
  color: ${({theme:e})=>e.systemColors.black};
  margin-bottom: ${({theme:e})=>e.spacing.sp4};
`,gs=({src:e,alt:t="",placement:n="top",children:r})=>{const a=(0,o.useModalPlaceContext)(),i="string"==typeof a?"undefined"!=typeof document?document.body:null:a,[l,s]=(0,D.useState)(!1),[u,c]=(0,D.useState)(null),d=(0,D.useRef)(null),p=(0,D.useRef)(null),g=(0,D.useCallback)((()=>{if(!d.current)return;const e=d.current.getBoundingClientRect(),t=e.left+e.width/2-104;c("top"===n?{left:t,top:e.top-208-8-10}:{left:t,top:e.bottom+8+10})}),[n]);(0,D.useLayoutEffect)((()=>{if(l)return g(),window.addEventListener("scroll",g,!0),window.addEventListener("resize",g),()=>{window.removeEventListener("scroll",g,!0),window.removeEventListener("resize",g)};c(null)}),[l,g]),(0,D.useEffect)((()=>()=>{p.current&&clearTimeout(p.current)}),[]);const h=()=>{p.current&&(clearTimeout(p.current),p.current=null),s(!0)},m=()=>{p.current=setTimeout((()=>s(!1)),0)},f=null!=u?u:{top:0,left:0},v=l&&i?(0,E.jsx)(vs,{$top:f.top,$left:f.left,$placement:n,onMouseEnter:h,onMouseLeave:m,children:(0,E.jsx)(ys,{src:e,alt:t})}):null;return(0,E.jsxs)(hs,{ref:d,onMouseEnter:h,onMouseLeave:m,children:[i&&(0,Uo.createPortal)(v,i),r]})},hs=B.default.div`
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  max-width: 100%;
  max-height: 100%;
`,ms=B.css`
  bottom: -8px;
  top: auto;
  border-left: 1px solid #a6b7c9;
  border-bottom: 1px solid #a6b7c9;
  border-right: none;
  border-top: none;
`,fs=B.css`
  top: -8px;
  bottom: auto;
  border-right: 1px solid #a6b7c9;
  border-top: 1px solid #a6b7c9;
  border-left: none;
  border-bottom: none;
`,vs=B.default.div`
  position: fixed;
  top: ${({$top:e})=>e}px;
  left: ${({$left:e})=>e}px;
  z-index: 2147483647;
  width: ${208}px;
  padding: 4px;
  background-color: #fff;
  border: 1px solid #a6b7c9;
  border-radius: 8px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);

  &::after {
    content: "";
    position: absolute;
    left: 50%;
    margin-left: -7px;
    width: 14px;
    height: 14px;
    background-color: #fff;
    transform: rotate(-45deg);
    z-index: 0;
    ${({$placement:e})=>"top"===e?ms:fs}
  }
`,ys=B.default.img`
  position: relative;
  z-index: 1;
  display: block;
  width: 200px;
  height: 200px;
  max-width: 200px;
  max-height: 200px;
  object-fit: contain;
  border-radius: 8px;
`,ks=(0,D.memo)((({review:e,highlightText:t,highlightPhrases:n})=>{const{t:r}=(0,o.useTranslation)(),[a,i]=(0,D.useState)(!1),[l,s]=(0,D.useState)(!1),u=(0,D.useRef)(null),c=(0,D.useMemo)((()=>(null==n?void 0:n.length)||t?bs(e.body,null!=n?n:[],t):e.body),[e.body,t,n]);(0,D.useLayoutEffect)((()=>{if(a)return;const e=u.current;e&&s(e.scrollHeight>e.clientHeight+2)}),[a,e.body,t,n]);return(0,E.jsxs)(Cs,{children:[(0,E.jsxs)(Ms,{children:[(0,E.jsxs)(Ls,{children:[e.avatarUrl?(0,E.jsx)(js,{src:e.avatarUrl,alt:e.author}):(0,E.jsx)(Ss,{children:e.author.charAt(0)}),(0,E.jsxs)(Is,{children:[(0,E.jsx)(Ts,{children:e.author}),(0,E.jsxs)(Ps,{children:[e.country," \xb7 ",e.date,e.variation&&(0,E.jsxs)(E.Fragment,{children:[" \xb7 ",(0,E.jsxs)(Es,{onClick:()=>{if(!e.skuId)return;const t=new URL(window.location.href);t.searchParams.set("sku_id",e.skuId),window.open(t.toString(),"_blank")},children:[e.variationImageUrl&&(0,E.jsx)(Ds,{src:e.variationImageUrl,alt:""}),e.variation,(0,E.jsx)(A.G,{icon:O.nUK,className:"variation-link-icon"})]})]})]})]})]}),(0,E.jsx)(eo.iG,{value:e.rating,noText:!0})]}),(0,E.jsxs)(Os,{children:[(0,E.jsx)(As,{ref:u,$expanded:a,children:c}),!a&&l&&(0,E.jsxs)(Ns,{type:"button",onClick:()=>{ei("seeMore"),i(!0)},children:["... ",r("tiktok.reviewInsights.reviews.seeMore")]}),a&&l&&(0,E.jsx)(Bs,{type:"button",onClick:()=>i(!1),children:r("tiktok.reviewInsights.reviews.seeLess")})]}),e.images.length>0&&(0,E.jsx)($s,{children:e.images.map((e=>(0,E.jsx)(gs,{src:e,children:(0,E.jsx)(Gs,{src:e,alt:""})},e)))}),e.helpfulCount>0&&(0,E.jsxs)(Fs,{children:[(0,E.jsx)(zs,{children:"\u{1f44d}"}),(0,E.jsx)(Us,{children:r("tiktok.reviewInsights.reviews.foundHelpful",{value:e.helpfulCount})})]})]})})),xs=B.default.mark`
  background-color: #CAF4B9;
  padding: 0 2px;
`,ws=e=>{const t=e.trim().split(/\s+/).filter(Boolean).map((e=>ma(e)));return 0===t.length?"":`${t.join("\\s+")}\\b`},bs=(e,t,n)=>{const r=[...t.filter(Boolean),...n?[n]:[]].filter(((e,t,n)=>n.indexOf(e)===t));if(0===r.length)return e;const o=r.map(ws).filter(Boolean).join("|");if(!o)return e;const a=new RegExp(`(${o})`,"gi"),i=[];let l=0,s=0;for(let t=a.exec(e);null!==t;t=a.exec(e)){t.index>l&&i.push(e.slice(l,t.index));const n=t[0];i.push((0,E.jsx)(xs,{children:n},s++)),l=a.lastIndex}return 0===l?e:(l<e.length&&i.push(e.slice(l)),i)},Cs=B.default.div`
  display: flex;
  flex-direction: column;
  padding: ${({theme:e})=>e.spacing.sp16};

  &:not(:last-of-type) {
    border-bottom: 1px solid ${({theme:e})=>e.systemColors.grey200};
  }
`,Ms=B.default.div`
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: ${({theme:e})=>e.spacing.sp8};
`,Ls=B.default.div`
  display: flex;
  align-items: center;
  gap: ${({theme:e})=>e.spacing.sp8};
`,Ss=B.default.div`
  width: 32px;
  height: 32px;
  border-radius: 50%;
  background-color: ${({theme:e})=>e.systemColors.grey200};
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 14px;
  color: ${({theme:e})=>e.systemColors.grey600};
  flex-shrink: 0;
`,js=B.default.img`
  width: 32px;
  height: 32px;
  border-radius: 50%;
  object-fit: cover;
  flex-shrink: 0;
  border: 1px solid ${({theme:e})=>e.systemColors.grey200};
`,Is=B.default.div`
  display: flex;
  flex-direction: column;
`,Ts=B.default.span`
  ${Ie.nn};
  color: ${({theme:e})=>e.systemColors.black};
`,Ps=B.default.span`
  ${Ie.nO};
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  white-space: pre-wrap;
  color: ${({theme:e})=>e.systemColors.grey600};
`,Es=B.default.span`
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: ${({theme:e})=>e.systemColors.grey800};
  cursor: pointer;
  
  &:hover {
    .variation-link-icon {
      color: ${({theme:e})=>e.systemColors.blue600};
    }
  }

  .variation-link-icon {
    font-size: 10px;
    color: ${({theme:e})=>e.systemColors.grey400};
    transition: color 0.2s ease;
  }
`,Ds=B.default.img`
  width: 16px;
  height: 16px;
  border-radius: 50%;
  object-fit: cover;
  border: 1px solid ${({theme:e})=>e.systemColors.grey200};
`,Os=B.default.div`
  position: relative;
  margin-bottom: ${({theme:e})=>e.spacing.sp8};
  ${Ie.nO};
  color: ${({theme:e})=>e.systemColors.grey800};
  min-width: 0;
`,As=B.default.div`
  min-width: 0;
  line-height: ${18}px;
  max-height: ${({$expanded:e})=>e?"none":"54px"};
  
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: ${({$expanded:e})=>e?"unset":3};
  overflow: hidden;
  word-break: break-word;
`,Rs=B.default.button`
  padding: 0;
  border: none;
  background: none;
  font: inherit;
  color: ${({theme:e})=>e.systemColors.blue600};
  cursor: pointer;
  &:hover {
    text-decoration: underline;
  }
`,Ns=(0,B.default)(Rs)`
  position: absolute;
  bottom: 0;
  right: 0;
  
  padding-left: 32px;
  padding-right: 2px;
  
  background: linear-gradient(
    to right, 
    rgba(255, 255, 255, 0) 0%, 
    rgba(255, 255, 255, 1) 25%, 
    rgba(255, 255, 255, 1) 100%
  );
  
  white-space: nowrap;
  line-height: ${18}px;
  font-weight: 500;
`,Bs=(0,B.default)(Rs)`
  display: inline-block;
  margin-top: ${({theme:e})=>e.spacing.sp4};
`,$s=B.default.div`
  display: flex;
  gap: ${({theme:e})=>e.spacing.sp8};
  margin-bottom: ${({theme:e})=>e.spacing.sp8};
`,Gs=B.default.img`
  width: 64px;
  height: 64px;
  object-fit: cover;
  border-radius: 4px;
  border: 1px solid ${({theme:e})=>e.systemColors.grey200};
  cursor: pointer;
`,Fs=B.default.div`
  display: flex;
  align-items: center;
  gap: ${({theme:e})=>e.spacing.sp4};
`,zs=B.default.span`
  font-size: 14px;
`,Us=B.default.span`
  ${Ie.nO};
  color: ${({theme:e})=>e.systemColors.blue600};
`,_s=()=>{const{t:e}=(0,o.useTranslation)(),t=Ga((e=>e.reviews.length)),n=Ga(Sa),r=Ga((e=>e.selectedPhrases));return 0===n.length?t>0?(0,E.jsx)(Qi,{}):(0,E.jsx)(Vi,{children:e("tiktok.reviewInsights.reviews.noReviews")}):(0,E.jsx)(E.Fragment,{children:n.map((e=>(0,E.jsx)(ks,{review:e,highlightPhrases:r},e.id)))})},Ws={all:"reviews",keywords:"keywords",variations:"variations"},Vs=e=>["=","+","-","@","\t","\r"].some((t=>e.startsWith(t)))?`'${e}`:e,Hs=e=>e.includes(",")||e.includes('"')||e.includes("\n")?`"${e.replace(/"/g,'""')}"`:e,Zs=["January","February","March","April","May","June","July","August","September","October","November","December"],Ks=(e,t)=>{const n=(null==t?void 0:t.trim())||"the United States";if(!(null==e?void 0:e.trim()))return`Reviewed in ${n} on`;const r=new Date(e.trim());if(Number.isNaN(r.getTime()))return`Reviewed in ${n} on ${e}`;return`Reviewed in ${n} on ${Zs[r.getMonth()]} ${r.getDate()}, ${r.getFullYear()}`},Qs=(e,t)=>{var n;const r=null!=(n=null==e?void 0:e.trim())?n:"";if(!r)return"-";if(t.length>0){const e=[],n=r.toLowerCase();for(const{label:r,options:o}of t){const t=o.filter((e=>n.includes(e.trim().toLowerCase()))).sort(((e,t)=>t.length-e.length))[0];t&&e.push(`${r}: ${t.trim()}`)}if(e.length>0)return e.join(" | ")}const o=r.split(/\s*\|\s*|\uff5c|,\s*/).map((e=>e.trim())).filter(Boolean);return 0===o.length?"-":o.map((e=>/:/.test(e)?e:`Variation: ${e}`)).join(" | ")},Ys="Date,Author,Verified,Body,Rating,Images,Videos,Variation",Xs=e=>[Hs(Vs(e.phrase)),String(e.reviewIds.length),null!=e.rating?String(e.rating):"-"].join(","),qs="Keyword Phrase,Appeared,Rating",Js=e=>{const t=[...new Set(e.reviews.map((e=>{var t;return(null!=(t=e.skuId)?t:"").trim()})).filter(Boolean))];return 0===t.length?"-":Hs(t.join("; "))},eu="Product Id,Description,Review Count,Review Share,Rating",tu=(e,t=[])=>[eu,...e.map((e=>((e,t)=>[Js(e),Hs(Vs(Qs(e.variation,t))),String(e.reviews.length),`${e.reviewShare}%`,String(e.avgRating)].join(","))(e,t)))].join("\n"),nu=(e,t=[])=>[Ys,...e.map((e=>((e,t)=>{var n;return[Hs(Vs(Ks(e.date,e.country))),Hs(Vs(e.author)),e.isVerifiedPurchase?"yes":"-",Hs(Vs(e.body)),String(e.rating),(null==(n=e.images)?void 0:n.length)?e.images.join("; "):"-","-",Hs(Vs(Qs(e.variation,t)))].join(",")})(e,t)))].join("\n"),ru=(e,t)=>{let n="";const r=(new Date).toISOString().slice(0,10);return n=t?"all"===e?`${t} ${r}`:`${t} ${e} ${r}`:`tiktok-${Ws[e]}-${r}`,n.replace(/[\/\\:*?"<>|]/g,"_")},ou=(e,t,n)=>{((e,t)=>{const n=new Blob([`\ufeff${e}`],{type:"text/csv;charset=utf-8;"}),r=URL.createObjectURL(n),o=document.createElement("a");o.href=r,o.download=`${t}.csv`,o.click(),URL.revokeObjectURL(r)})(e,ru(t,(e=>e?e.slice(0,150):"")(n)))},au=(e,t)=>{var n;if(0===e.length)return;const r=nu(e,null!=(n=null==t?void 0:t.variationGroups)?n:[]);ou(r,"all",null==t?void 0:t.title)},iu=(e,t)=>{0!==e.length&&ou((e=>[qs,...e.map(Xs)].join("\n"))(e),"keywords",null==t?void 0:t.title)},lu=()=>{const{t:e}=(0,o.useTranslation)(),t=Ga((e=>e.reviews.length)),n=Ga(ja),r=Ga((e=>Sa(e).length)),a=Ga((e=>e.reviews)),i=Ga((e=>e.productTitle)),l=Ga((e=>e.variationGroups)),s=Ga((e=>e.selectedPhrases)),[u,c]=(0,D.useState)(new Set),d=(0,D.useMemo)((()=>s.length>0?n.filter((e=>s.includes(e.phrase))):n),[n,s]);(0,D.useEffect)((()=>{s.length>0&&d.length>0?c(new Set([0])):0===s.length&&c(new Set)}),[s.length,d.length]);const p=(0,D.useMemo)((()=>new Map(a.map((e=>[e.id,e])))),[a]),g=(0,D.useCallback)((e=>{c((t=>{const n=new Set(t);return n.has(e)?n.delete(e):n.add(e),n}))}),[]),h=(0,D.useCallback)((e=>t=>{t.stopPropagation();const n=e.map((e=>p.get(e))).filter(Boolean);au(n,{productId:gi(),title:i||void 0,variationGroups:l})}),[p,i,l]);return 0===n.length?t>0&&0===r?(0,E.jsx)(Qi,{}):(0,E.jsx)(Vi,{children:e("tiktok.reviewInsights.reviews.noReviews")}):0===d.length?(0,E.jsx)(Vi,{children:e("tiktok.reviewInsights.reviews.noReviews")}):(0,E.jsx)(E.Fragment,{children:d.map(((t,n)=>(0,E.jsxs)(su,{children:[(0,E.jsxs)(uu,{onClick:()=>g(n),children:[(0,E.jsxs)(cu,{children:[(0,E.jsx)(je.R,{buttonType:"icon",icon:(0,E.jsx)(A.G,{icon:O.ptq,rotation:u.has(n)?180:void 0})}),t.phrase," (",t.reviewIds.length,")"]}),(0,E.jsxs)(du,{children:[(0,E.jsx)(pu,{children:null!==t.rating&&(0,E.jsx)(eo.iG,{value:t.rating})}),(0,E.jsx)(je.R,{buttonType:"text",icon:(0,E.jsx)(A.G,{icon:O.ca9}),onClick:h(t.reviewIds),style:{color:"#485E75"},children:e("tiktok.reviewInsights.export")})]})]}),u.has(n)&&(0,E.jsx)(Hi,{$borderTop:!0,children:t.reviewIds.map((e=>{const n=p.get(e);return n?(0,E.jsx)(ks,{review:n,highlightText:t.phrase,highlightPhrases:s},e):null}))})]},t.phrase)))})},su=B.default.div`
  width: 100%;

  &:not(:last-of-type) {
    border-bottom: solid 1px ${({theme:e})=>e.systemColors.grey200};
  }
`,uu=B.default.div`
  width: 100%;
  height: 40px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  cursor: pointer;
  padding: ${({theme:e})=>`0 ${e.spacing.sp16}`};
`,cu=B.default.div`
  ${Ie.nO};
  display: flex;
  align-items: center;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;

  svg {
    transition: transform 0.3s;
  }

  ${je.R} {
    margin-right: ${({theme:e})=>e.spacing.sp16};
  }
`,du=B.default.div`
  display: flex;
  flex-direction: row;
  align-items: center;
`,pu=B.default.div`
  margin: ${({theme:e})=>`0 ${e.spacing.sp16} 0 ${e.spacing.sp32}`};
`,gu=()=>{const{t:e}=(0,o.useTranslation)(),t=Ga((e=>e.reviews.length)),n=Ga((e=>Sa(e).length)),r=Ga(Ta),a=Ga((e=>e.variationGroups)),i=Ga((e=>e.productTitle)),l=Ga((e=>e.selectedPhrases)),[s,u]=(0,D.useState)(new Set),c=(0,D.useCallback)((e=>{u((t=>{const n=new Set(t);return n.has(e)?n.delete(e):n.add(e),n}))}),[]),d=(0,D.useCallback)((e=>t=>{t.stopPropagation(),au(e,{productId:gi(),title:i||void 0,variationGroups:a})}),[i,a]);return 0===r.length?t>0&&0===n?(0,E.jsx)(Qi,{}):(0,E.jsx)(Vi,{children:e("tiktok.reviewInsights.reviews.noReviews")}):(0,E.jsx)(E.Fragment,{children:r.map(((t,n)=>{var r;const o=s.has(n),i=((e,t,n,r)=>{const o=e.trim();if(!o)return[n];if(t.length>0){const e=[];for(const{label:n,options:r}of t){const t=r.filter((e=>o.includes(e.trim()))).sort(((e,t)=>t.length-e.length))[0];t&&e.push(`${n}: ${t.trim()}`)}if(e.length>0)return e}const a=o.split(/\s*\|\s*|\uff5c/).map((e=>e.trim())).filter(Boolean);return 0===a.length?[n]:a.map((e=>/:/.test(e)?e:`${r}: ${e}`))})(t.variation||"",a,e("tiktok.reviewInsights.variations.unknown"),e("tiktok.reviewInsights.variations.variationLabel"));return(0,E.jsxs)(hu,{children:[(0,E.jsxs)(mu,{$expanded:o,onClick:()=>c(n),children:[(0,E.jsxs)(fu,{children:[(0,E.jsx)(je.R,{buttonType:"icon",icon:(0,E.jsx)(A.G,{icon:O.ptq,rotation:o?180:void 0})}),(null==(r=t.reviews[0])?void 0:r.variationImageUrl)&&(0,E.jsx)(vu,{src:t.reviews[0].variationImageUrl,alt:""}),(0,E.jsxs)(yu,{children:[(0,E.jsx)(ku,{children:i.map(((e,t)=>(0,E.jsxs)(D.Fragment,{children:[(0,E.jsx)(xu,{children:e}),t<i.length-1&&(0,E.jsx)(wu,{})]},e)))}),(0,E.jsxs)(bu,{children:[(0,E.jsx)(eo.iG,{value:t.avgRating}),(0,E.jsx)(Cu,{children:` | ${e("tiktok.reviewInsights.variations.count")}: ${t.reviews.length} | ${e("tiktok.reviewInsights.variations.share")}: ${t.reviewShare}%`})]})]})]}),(0,E.jsx)(je.R,{buttonType:"text",icon:(0,E.jsx)(A.G,{icon:O.ca9}),onClick:d(t.reviews),style:{color:"#485E75"},children:e("tiktok.reviewInsights.export")})]}),o&&(0,E.jsx)(Hi,{children:t.reviews.map((e=>(0,E.jsx)(ks,{review:e,highlightPhrases:l},e.id)))})]},t.variation||"__none__")}))})},hu=B.default.div`
  width: 100%;

  &:not(:last-of-type) {
    border-bottom: solid 1px ${({theme:e})=>e.systemColors.grey220};
  }
`,mu=B.default.div`
  width: 100%;
  display: flex;
  justify-content: space-between;
  align-items: center;
  cursor: pointer;
  padding: ${({theme:e})=>`${e.spacing.sp8} ${e.spacing.sp16}`};
  ${({$expanded:e})=>e&&B.css`
      border-bottom: 1px solid ${({theme:e})=>e.systemColors.grey220};
    `};
`,fu=B.default.div`
  display: flex;
  gap: ${({theme:e})=>e.spacing.sp16};
  align-items: center;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;

  svg {
    transition: transform 0.3s;
  }
`,vu=B.default.img`
  width: 40px;
  height: 40px;
  border-radius: ${({theme:e})=>e.borderRadius.small};
  object-fit: cover;
  flex-shrink: 0;
`,yu=B.default.div`
  display: flex;
  flex-direction: column;
  gap: 2px;
`,ku=B.default.div`
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 0;
  min-width: 0;
`,xu=B.default.span`
  ${Ie.nn};
  color: ${({theme:e})=>e.systemColors.black};
`,wu=B.default.span`
  margin: 0 ${({theme:e})=>e.spacing.sp4};
  color: #485E75;
  font-weight: 600;
  user-select: none;
  flex-shrink: 0;

  &::before {
    content: "|";
  }
`,bu=B.default.div`
  display: flex;
  align-items: center;
`,Cu=B.default.span`
  ${Ie.nO};
  color: ${({theme:e})=>e.systemColors.grey600};
  margin-left: ${({theme:e})=>e.spacing.sp4};
`,Mu=()=>{switch(Ga((e=>e.view))){case"keywords":return(0,E.jsx)(lu,{});case"variations":return(0,E.jsx)(gu,{});default:return(0,E.jsx)(_s,{})}},Lu=["recommended","recent"],Su=()=>{const{t:e}=(0,o.useTranslation)(),t=Ga((e=>e.view)),n=Ga((e=>e.selectedPhrases)),r=Ga((e=>e.sortBy)),a=Ga((e=>e.setSortBy)),i=Ga((e=>e.reviews.length)),l=Ga(Sa),s=Ga((e=>e.productTitle)),u=Ga((e=>e.variationGroups)),c=l.length,d=Ga((e=>e.isLoading))||0===i,p=Ga((e=>e.wordCount)),g=Ga((e=>e.setWordCount)),h=Ga("keywords"===t?ja:()=>[]),m=Ga("variations"===t?Ta:()=>[]),f=(0,D.useMemo)((()=>n.length>0?h.filter((e=>n.includes(e.phrase))):h),[h,n]),v=(0,D.useMemo)((()=>"all"===t&&n.length>0?e("tiktok.reviewInsights.views.all.titleWithPhrase",{phrase:n.join(", ")}):e(`tiktok.reviewInsights.views.${t}.title`)),[t,n,e]),y=(0,D.useMemo)((()=>Lu.map((t=>({value:t,label:e(`tiktok.reviewInsights.sort.options.${t}`)})))),[e]),k=(0,D.useMemo)((()=>[2,3,4,5].map((e=>({value:e,label:String(e)})))),[]),x=(0,D.useCallback)((({value:e})=>{a(e)}),[a]),w=(0,D.useCallback)((({value:e})=>{g(e)}),[g]),b=(0,D.useCallback)((()=>{ei("export");const e=s||void 0;switch(t){case"keywords":iu(f,{title:e});break;case"variations":((e,t)=>{var n;if(0===e.length)return;const r=tu(e,null!=(n=null==t?void 0:t.variationGroups)?n:[]);ou(r,"variations",null==t?void 0:t.title)})(m,{title:e,variationGroups:u});break;case"all":au(l,{productId:gi(),title:e,variationGroups:u})}}),[t,f,m,l,s,u]);return(0,E.jsxs)(ju,{$isKeywordsView:"keywords"===t,children:[(0,E.jsxs)(Iu,{children:[(0,E.jsx)(no.u,{colorType:"light",content:v,children:(0,E.jsx)(Eu,{children:v})}),(0,E.jsx)(Du,{children:e("tiktok.reviewInsights.reviews.count",{filtered:c,total:i})})]}),(0,E.jsxs)(Tu,{$isKeywordsView:"keywords"===t,children:["keywords"===t&&(0,E.jsxs)(Pu,{children:[(0,E.jsx)(Ou,{children:e("tiktok.reviewInsights.wordCount.label")}),(0,E.jsx)(Fe.T,{value:p,options:k,onChange:w,disabled:d})]}),(0,E.jsxs)(Pu,{children:[(0,E.jsx)(Ou,{children:e("tiktok.reviewInsights.sort.label")}),(0,E.jsx)(Fe.T,{value:r,options:y,onChange:x,disabled:d})]}),(0,E.jsx)(je.R,{icon:(0,E.jsx)(A.G,{icon:O.ca9}),buttonType:"tertiary",disabled:d,onClick:b,children:e("tiktok.reviewInsights.export")})]})]})},ju=B.default.div`
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: ${({theme:e})=>`${e.spacing.sp16}`};
  border-bottom: 1px solid ${({theme:e})=>e.systemColors.grey220};

  ${({$isKeywordsView:e})=>e&&"\n    flex-direction: column;\n    align-items: stretch;\n  "}
`,Iu=B.default.div`
  display: flex;
  flex-direction: column;
  min-width: 0;
  overflow: hidden;
`,Tu=B.default.div`
  display: flex;
  gap: ${({theme:e})=>e.spacing.sp8};
  align-items: center;

  ${({$isKeywordsView:e})=>e&&"\n    justify-content: flex-end;\n  "}
`,Pu=B.default.div`
  display: flex;
  gap: ${({theme:e})=>e.spacing.sp8};
  align-items: center;
`,Eu=B.default.h2`
  ${Ie.nn};
  color: ${({theme:e})=>e.systemColors.black};
  max-width: 800px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
`,Du=B.default.div`
  ${Ie.nO};
  color: ${({theme:e})=>e.systemColors.grey600};
`,Ou=B.default.span``,Au=({isLimitReached:e,used:t,total:n,onUpgrade:r})=>{const o=Ga((e=>e.isLoading)),a=Ga((e=>e.reviews.length>0)),{setStatus:i}=(0,qa.A)(),l=o&&!a,s=o&&a&&!e;return(0,D.useEffect)((()=>{i(o?"start":"finish")}),[o,i]),(0,E.jsxs)(Ru,{children:[(0,E.jsxs)(Nu,{children:[(0,E.jsx)(ds,{}),(0,E.jsx)(Xl,{}),(0,E.jsx)(el,{})]}),(0,E.jsxs)(Bu,{children:[(0,E.jsx)(Su,{}),l?(0,E.jsx)($u,{children:(0,E.jsx)(rs,{})}):(0,E.jsxs)(E.Fragment,{children:[(0,E.jsxs)($u,{$isLimitReached:e,children:[e?(0,E.jsx)(Di,{used:t,total:n,onUpgrade:r}):(0,E.jsx)(Mu,{}),s&&(0,E.jsx)(Gu,{children:(0,E.jsx)(rs,{})})]}),!e&&(0,E.jsx)(Mi,{})]})]})]})},Ru=B.default.div`
  display: flex;
  gap: ${({theme:e})=>e.spacing.sp24};
  overflow: hidden;
  flex-grow: 1;

  div {
    ${Ja.pb}
  }
`,Nu=B.default.div`
  display: flex;
  flex-direction: column;
  width: 314px;
  overflow: auto;
  scrollbar-gutter: stable;
  gap: ${({theme:e})=>e.spacing.sp24};
  flex-shrink: 0;
`,Bu=(0,B.default)(to.Q)`
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
  padding: 0;
`,$u=B.default.div`
  position: relative;
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  ${({$isLimitReached:e})=>e&&"\n      display: flex;\n      align-items: center;\n      justify-content: center;\n    "}
`,Gu=B.default.div`
  position: absolute;
  inset: 0;
  z-index: 10;
  display: flex;
  align-items: center;
  justify-content: center;
  pointer-events: all;
  background: ${({theme:e})=>{var t;return null!=(t=e.systemColors.white)?t:"#fff"}};
`;var Fu=n(56783);const zu=()=>{const{t:e}=(0,o.useTranslation)();return(0,E.jsxs)(Uu,{children:[(0,E.jsx)(_u,{}),(0,E.jsx)(Wu,{children:e("tiktok.reviewInsights.title")})]})},Uu=B.default.div`
  display: flex;
  gap: ${({theme:e})=>e.spacing.sp8};
`,_u=(0,B.default)(Fu.r)`
  width: 20px;
  height: 20px;
`,Wu=B.default.div`
  ${Ie.uh};
  color: ${({theme:e})=>e.systemColors.blue900};
`;var Vu;function Hu(){return Hu=Object.assign?Object.assign.bind():function(e){for(var t=1;t<arguments.length;t++){var n=arguments[t];for(var r in n)({}).hasOwnProperty.call(n,r)&&(e[r]=n[r])}return e},Hu.apply(null,arguments)}const Zu=e=>D.createElement("svg",Hu({xmlns:"http://www.w3.org/2000/svg",width:14,height:11,fill:"none"},e),Vu||(Vu=D.createElement("path",{fill:"#485E75",d:"M2.557 1.38a.39.39 0 0 1 .348-.213h7.41c.149 0 .282.082.348.214l1.38 2.763a.08.08 0 0 1 .01.041.092.092 0 0 1-.092.093H1.259a.092.092 0 0 1-.092-.093.08.08 0 0 1 .01-.04l1.38-2.764ZM0 4.186C0 4.85.515 5.393 1.167 5.44V9.72c0 .644.522 1.166 1.166 1.166H7c.644 0 1.167-.522 1.167-1.166V5.442h2.722v4.861a.582.582 0 1 0 1.167 0V5.44a1.258 1.258 0 0 0 1.033-1.818L11.707.86A1.557 1.557 0 0 0 10.315 0H2.907c-.588 0-1.128.333-1.39.86L.134 3.624A1.258 1.258 0 0 0 0 4.188v-.003ZM2.333 7.39V5.444H7V7.39H2.333Zm0 1.167H7v1.166H2.333V8.556Z"})));var Ku,Qu,Yu,Xu,qu;function Ju(){return Ju=Object.assign?Object.assign.bind():function(e){for(var t=1;t<arguments.length;t++){var n=arguments[t];for(var r in n)({}).hasOwnProperty.call(n,r)&&(e[r]=n[r])}return e},Ju.apply(null,arguments)}const ec=e=>D.createElement("svg",Ju({xmlns:"http://www.w3.org/2000/svg",width:11,height:11,fill:"none"},e),Ku||(Ku=D.createElement("path",{fill:"#EE1D52",d:"m4.07 7.81.055.11s0-.056-.055-.11Zm-1.65-.716c.055-.55.275-.88.66-1.155.605-.44 1.32-.165 1.32-.165v-1.43c.165 0 .33 0 .55.055v1.815s-.715-.22-1.32.165c-.385.33-.605.605-.66 1.155 0 .275.055.66.275 1.045-.055-.055-.11-.055-.165-.11-.55-.385-.66-.99-.66-1.375Zm5.5-5.39A2.623 2.623 0 0 1 7.315.44h.495s-.11.825.605 1.705c-.165-.11-.33-.275-.495-.44Z"})),Qu||(Qu=D.createElement("path",{fill:"#EE1D52",d:"M10.34 2.97v1.814s-.66 0-1.1-.165c-.66-.165-1.1-.44-1.1-.44s-.275-.22-.33-.22V7.7c0 .22-.055.715-.22 1.155-.22.55-.55.935-.605.99 0 0-.385.495-1.1.825-.66.33-1.21.33-1.375.33 0 0-.99.055-1.87-.55a3.858 3.858 0 0 1-.55-.44c.88.605 1.87.55 1.87.55.165 0 .715 0 1.375-.33.715-.33 1.1-.825 1.1-.825.055-.055.385-.44.605-.99.165-.44.22-.935.22-1.155V3.52l.33.22s.44.275 1.1.44c.495.11 1.1.165 1.1.165v-1.43c.22.056.44.056.55.056Z"})),Yu||(Yu=D.createElement("path",{fill:"#030001",d:"M9.845 2.915v1.43s-.66 0-1.1-.165c-.66-.165-1.1-.44-1.1-.44s-.275-.22-.33-.22v3.74c0 .22-.055.714-.22 1.154-.22.55-.55.935-.605.99 0 0-.385.495-1.1.825-.66.33-1.21.275-1.375.33 0 0-.99.055-1.87-.55-.11-.11-.165-.22-.274-.275-.33-.385-.495-.825-.55-.935-.055-.22-.22-.715-.22-1.21.055-.88.33-1.375.385-1.54.22-.385.494-.66.77-.935.274-.22.604-.44.934-.55.275-.11.495-.165.715-.165.165-.054.33-.054.495-.054v1.43s-.715-.22-1.32.164c-.385.33-.605.605-.66 1.155 0 .385.11.99.66 1.375.055.055.11.055.165.11l.33.33c.55.33.99.385 1.54.165.385-.165.66-.495.77-.88.11-.22.055-.495.055-.715V.44h1.32c.055.33.22.77.605 1.266.165.165.33.33.55.44.055.055.33.385.715.55.275.11.495.165.715.22Z"})),Xu||(Xu=D.createElement("path",{fill:"#69C9D0",d:"m.826 8.359.055.11c-.055-.055-.055-.055-.055-.11Zm2.42-3.795c-.33.11-.66.33-.936.55a6.33 6.33 0 0 0-.77.935c-.054.11-.384.66-.384 1.54 0 .495.11.99.22 1.21.055.11.22.55.495.88.054.11.164.22.274.275-.274-.22-.55-.44-.77-.715-.274-.33-.44-.77-.494-.88-.055-.22-.22-.715-.22-1.21.055-.88.33-1.375.385-1.54.165-.33.44-.66.77-.935.274-.22.604-.44.934-.55.22-.11.44-.164.66-.164.33-.11.66-.11.99-.055v.384c-.165 0-.33 0-.495.055-.22.055-.44.11-.66.22Z"})),qu||(qu=D.createElement("path",{fill:"#69C9D0",d:"M7.315.44h-1.32v6.984c0 .22 0 .495-.055.715-.11.385-.44.715-.77.88-.55.22-.99.22-1.54-.165l-.33-.33c.385.275.825.275 1.32.055.385-.165.66-.495.77-.88.11-.22.055-.495.055-.715V0H7.26c.055 0 0 .165.055.44Zm2.53 2.09v.385c-.22-.055-.44-.11-.605-.22-.385-.165-.66-.495-.715-.55 0 .055.055.055.165.11.44.22.88.33 1.155.275Z"}))),tc=({product:e})=>(0,E.jsxs)(nc,{children:[(0,E.jsxs)(rc,{children:[(0,E.jsx)(gs,{src:e.imageUrl,alt:e.title,placement:"bottom",children:(0,E.jsx)(oc,{src:e.imageUrl,alt:e.title})}),(0,E.jsx)(ac,{children:(0,E.jsx)(ec,{})})]}),(0,E.jsxs)(ic,{children:[(0,E.jsx)(lc,{children:e.title}),(0,E.jsxs)(sc,{children:[(0,E.jsx)(uc,{children:(0,E.jsx)(Zu,{})}),(0,E.jsxs)(cc,{children:[e.shopName," \xb7 ",e.variationsCount," Variations",e.brand?` &middot; ${e.brand}`:""]})]})]})]}),nc=B.default.div`
  display: flex;
  gap: ${({theme:e})=>e.spacing.sp8};
  align-items: center;
  flex: 1;
  min-width: 0;
`,rc=B.default.div`
  position: relative;
  width: 48px;
  height: 48px;
  flex-shrink: 0;
  border-radius: 4px;
  border: 1px solid ${({theme:e})=>e.systemColors.grey200};
  background: ${({theme:e})=>e.systemColors.white};
  display: flex;
  align-items: center;
  justify-content: center;
  padding: ${({theme:e})=>e.spacing.sp4};
`,oc=B.default.img`
  max-width: 100%;
  max-height: 100%;
  object-fit: contain;
  border-radius: 4px;
`,ac=B.default.div`
  position: absolute;
  bottom: 0;
  right: 0;
  width: 18px;
  height: 18px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: ${({theme:e})=>e.systemColors.white};

  svg {
    width: 11px;
    height: 11px;
  }
`,ic=B.default.div`
  display: flex;
  flex-direction: column;
  gap: ${({theme:e})=>e.spacing.sp4};
  overflow: hidden;
`,lc=B.default.div`
  ${Ie.xk};
  color: ${({theme:e})=>e.systemColors.grey800};
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
`,sc=B.default.div`
  display: flex;
  align-items: center;
  gap: ${({theme:e})=>e.spacing.sp4};
`,uc=B.default.div`
  width: 14px;
  height: 14px;
  color: ${({theme:e})=>e.systemColors.grey600};
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
`,cc=B.default.div`
  ${Ie.Qv};
  color: ${({theme:e})=>e.systemColors.grey600};
  white-space: nowrap;
  overflow: hidden;
  font-size: 14px;
  text-overflow: ellipsis;
`,dc=({distribution:e})=>(0,E.jsx)(pc,{children:e.map((e=>(0,E.jsxs)(gc,{children:[(0,E.jsxs)(hc,{children:[e.stars," ",1===e.stars?"star":"stars",":"]}),(0,E.jsx)(mc,{children:(0,E.jsx)(fc,{$percentage:e.percentage})}),(0,E.jsxs)(vc,{children:[e.percentage,"%"]})]},e.stars)))}),pc=B.default.div`
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
`,gc=B.default.div`
  display: flex;
  align-items: center;
  gap: ${({theme:e})=>e.spacing.sp8};
`,hc=B.default.span`
  ${Ie.Sv};
  color: ${({theme:e})=>e.systemColors.grey800};
  white-space: nowrap;
  width: 48px;
`,mc=B.default.div`
  width: 100px;
  height: 8px;
  border-radius: 2px;
  background-color: ${({theme:e})=>e.systemColors.grey100};
  overflow: hidden;
  flex-shrink: 0;
`,fc=B.default.div`
  height: 100%;
  width: ${({$percentage:e})=>e}%;
  background-color: ${({theme:e})=>e.systemColors.yellow400};
  border-radius: 2px;
`,vc=B.default.span`
  ${Ie.Sv};
  color: ${({theme:e})=>e.systemColors.grey800};
  width: 30px;
  text-align: left;
`,yc=({rating:e,reviewsCount:t})=>(0,E.jsxs)(kc,{children:[(0,E.jsxs)(xc,{children:[(0,E.jsx)(wc,{children:e.toFixed(1)}),(0,E.jsxs)(bc,{children:["(",t.toLocaleString(),")"]})]}),(0,E.jsx)(eo.iG,{value:e,noText:!0})]}),kc=B.default.div`
  display: flex;
  flex-direction: column;
  gap: ${({theme:e})=>e.spacing.sp4};
  flex-shrink: 0;
`,xc=B.default.div`
  display: flex;
  align-items: baseline;
  gap: ${({theme:e})=>e.spacing.sp8};
`,wc=B.default.span`
  ${Ie.Th};
`,bc=B.default.span`
  ${Ie.sh};
  color: ${({theme:e})=>e.systemColors.grey600};
`,Cc=({data:e})=>(0,E.jsxs)(Mc,{children:[(0,E.jsxs)(Lc,{children:[(0,E.jsx)(yc,{rating:e.rating,reviewsCount:e.reviewsCount}),(0,E.jsx)(dc,{distribution:e.ratingDistribution})]}),(0,E.jsx)(Sc,{children:(0,E.jsx)(tc,{product:e.product})})]}),Mc=B.default.div`
  display: flex;
  align-items: center;
  background-color: ${({theme:e})=>e.systemColors.grey50};
  padding: ${({theme:e})=>`${e.spacing.sp16} ${e.spacing.sp32}`};
  gap: ${({theme:e})=>e.spacing.sp24};
  width: 100%;
`,Lc=B.default.div`
  display: flex;
  align-items: flex-start;
  gap: ${({theme:e})=>e.spacing.sp24};
  flex-shrink: 0;
`,Sc=B.default.div`
  display: flex;
  flex: 1;
  min-width: 0;
`,jc=()=>{const{t:e}=(0,o.useTranslation)(),t=Ga((e=>e.loadedCount)),n=Ga((e=>e.totalCount)),r=null!=n?n:t;return(0,E.jsxs)(Ic,{children:[(0,E.jsx)(Tc,{children:e("tiktok.reviewInsights.banner.title",{loaded:t.toLocaleString(),total:r.toLocaleString()})}),(0,E.jsx)(Pc,{children:e("tiktok.reviewInsights.banner.subtitle")})]})},Ic=B.default.div`
  padding: ${({theme:e})=>`${e.spacing.sp12} ${e.spacing.sp32} 0 ${e.spacing.sp32}`};
  flex-shrink: 0;
`,Tc=B.default.div`
  ${Ie.nH};
  font-weight: 800;
  color: ${({theme:e})=>e.systemColors.black};
`,Pc=B.default.div`
  ${Ie.nO};
  color: ${({theme:e})=>e.systemColors.grey600};
  margin-top: 2px;
`,Ec=e=>Array.prototype.slice.call(e),Dc=e=>e.replace(/\D/g,""),Oc=()=>{try{const e=document.getElementById("pdp-review-section");if(e){const t=(e=>{var t,n,r,o,a,i,l;const s=e.children[0];if(!s)return null;const[u,c]=Ec(s.children);if(!u)return null;const d=u.querySelector(".H1-Bold"),p=null!=(n=null==(t=null==d?void 0:d.innerText)?void 0:t.trim())?n:"";let g="";const h=u.querySelector(".H2-Semibold");h&&(g=Dc(null!=(r=h.innerText)?r:""));const m=[];if(c){const e=Ec(c.children);for(const t of e){const e=t.querySelector(".H3-Bold"),n=t.querySelector(".H3-Regular");e&&n&&m.push({score:null!=(a=null==(o=e.innerText)?void 0:o.trim())?a:"",count:null!=(l=null==(i=n.innerText)?void 0:i.trim())?l:""})}}return{reviewOverallScore:p,reviewCount:g,reviewRatingResult:m}})(e);if(t)return t}const t=(()=>{var e;const t=document.querySelectorAll(".slick-slider-container")[0];if(!t)return null;const n=t.parentElement;return n&&null!=(e=n.nextElementSibling)?e:null})();return t?(e=>{var t,n,r,o;const[a]=Ec(e.children);if(!a)return null;const[i,l]=Ec(a.children);if(!i)return null;const[s,,,u]=Ec(i.children),c=null!=(t=null==s?void 0:s.innerText)?t:"",d=Dc(null!=(n=null==u?void 0:u.innerText)?n:""),p=[];if(l){const e=Ec(l.children);for(const t of e){const[e,,n]=Ec(t.children),[a]=Ec(e.children);p.push({score:null!=(r=null==a?void 0:a.innerText)?r:"",count:null!=(o=null==n?void 0:n.innerText)?o:""})}}return{reviewOverallScore:c,reviewCount:d,reviewRatingResult:p}})(t):null}catch(e){return null}},Ac=()=>{var e,t,n,r,o,a;const i=Oc();if(!i)return null;const l=(()=>{var e,t,n,r,o,a,i,l,s;try{const u=document.querySelector("h1"),c=null!=(t=null==(e=null==u?void 0:u.innerText)?void 0:e.trim())?t:"";let d="";const p=document.querySelector('.slick-slider-container img[data-fmp="true"]');if(p)d=p.src;else{const e=document.querySelector(".slick-slider-container img");d=null!=(n=null==e?void 0:e.src)?n:""}let g="";const h=document.querySelector('a[href*="/store/"]:has(img.w-72.h-72.rounded-full)');if(h){const e=h.querySelector("span.H2-Bold");g=null!=(o=null==(r=null==e?void 0:e.innerText)?void 0:r.trim())?o:""}let m="";const f=document.querySelectorAll("span.H3-Regular");for(const e of f){const t=(null!=(a=e.innerText)?a:"").match(/([\d,.]+K?)\s*sold/i);if(t){m=t[1].trim();break}}const v=[],y=document.querySelector(".sticky.overflow-y-auto");if(y){const e=y.querySelectorAll("[class*='overflow-x-auto'][class*='flex-wrap']");for(const t of e){let e="";const n=t.parentElement;if(n){const t=n.querySelector("span.H2-Semibold");e=null!=(l=null==(i=null==t?void 0:t.innerText)?void 0:i.trim().replace(/:$/,""))?l:""}const r=[],o=t.querySelectorAll("span.P3-Regular");for(const e of o){const t=null==(s=e.innerText)?void 0:s.trim();t&&r.push(t)}r.length>0&&v.push({label:e,options:r})}}return{title:c,imageUrl:d,shopName:g,soldCount:m,variationsCount:v.flatMap((e=>e.options)).length,variationGroups:v}}catch(e){return null}})(),s=Number.parseFloat(i.reviewOverallScore)||0,u=Number.parseInt(i.reviewCount,10)||0,c=i.reviewRatingResult.reduce(((e,t)=>e+(Number.parseInt(t.count,10)||0)),0);return{rating:s,reviewsCount:u,ratingDistribution:i.reviewRatingResult.map((e=>{const t=Number.parseInt(e.count,10)||0;return{stars:Number.parseInt(e.score,10)||0,percentage:c>0?Math.round(t/c*100):0}})),product:{title:null!=(e=null==l?void 0:l.title)?e:"",imageUrl:null!=(t=null==l?void 0:l.imageUrl)?t:"",shopName:null!=(n=null==l?void 0:l.shopName)?n:"",soldCount:null!=(r=null==l?void 0:l.soldCount)?r:"",variationsCount:null!=(o=null==l?void 0:l.variationsCount)?o:0,variationGroups:null!=(a=null==l?void 0:l.variationGroups)?a:[]}}},Rc=()=>{const[e,t]=(0,D.useState)(null);return(0,D.useEffect)((()=>{let e,n=null,r=!1;const o=()=>{if(r)return;const e=Ac();e&&(e=>{var n;r=!0,t(e);const o=Ga.getState();o.setProductTitle(null!=(n=e.product.title)?n:""),e.product.variationGroups.length>0&&o.setVariationGroups(e.product.variationGroups),a()})(e)},a=()=>{n&&(n.disconnect(),n=null),clearTimeout(e)};return o(),r||(n=new MutationObserver(o),n.observe(document.body,{childList:!0,subtree:!0}),e=setTimeout((()=>{a()}),3e4)),a}),[]),e},Nc=()=>{const[e,t]=(0,D.useState)(null),{isVisible:n}=(0,o.useModalState)("h10-tiktok-review-insights"),r=(0,D.useRef)(!1);return(0,D.useEffect)((()=>{n&&!r.current&&(r.current=!0,(0,o.updateLinkWithAccountId)(o.xrayInvocationLink).then((e=>(0,V.H)({url:e,type:o.backgroundMessageTypes.BACKGROUND_REQUEST,method:"POST",params:{appId:chrome.runtime.id,platform:"review-insights-tiktok"}}))).then(t).catch((()=>t(null))))}),[n]),e},Bc=({name:e})=>{var t;const n=Rc(),r=Nc(),a=403===(null==r?void 0:r.status),i=null!==r&&!a,{refetch:s}=((e,t)=>{const n=Ga((e=>e.setReviews)),r=Ga((e=>e.setAnalysis)),o=Ga((e=>e.setIsLoading)),a=Ga((e=>e.setIsLoadingMore)),i=Ga((e=>e.setTotalCount)),l=Ga((e=>e.setHasMore)),s=Ga((e=>e.sortBy)),u=(0,D.useCallback)((()=>{if(!e)return void(t&&(o(!1),a(!1)));a(!1),o(!0);const s=(e=>"recent"===e?"latest":"recommend")(Ga.getState().sortBy);ki({pageStart:1,pageSize:500,sortRule:s}).then((({has_more:e,product_reviews:t,review_ratings:o})=>{const a=ri(t);n(a),r(li(a)),l(e),(null==o?void 0:o.review_count)&&i(Number.parseInt(o.review_count,10)||null)})).catch((()=>{l(!1)})).finally((()=>{o(!1)}))}),[e,t,n,r,o,a,i,l]);return(0,D.useEffect)((()=>{u()}),[u,s]),{refetch:u}})(i&&null!=(t=null==n?void 0:n.reviewsCount)?t:0,i),u=Ga((e=>e.resetForSyncRefresh)),c=(0,D.useRef)(null),d=(0,D.useCallback)((()=>{null!=c.current&&(clearTimeout(c.current),c.current=null),c.current=setTimeout((()=>{c.current=null,u(),s()}),500)}),[s,u]);(0,D.useEffect)((()=>()=>{null!=c.current&&clearTimeout(c.current)}),[]);const p=!a&&null!==r&&null!==r.left&&null!==r.totalAvailable&&0!==r.totalAvailable,g=(p||a)&&null!=r.totalAvailable&&null!=r.left?r.totalAvailable-r.left:0,h=(p||a)&&null!=r.totalAvailable?r.totalAvailable:0,{openToolSpecific:m}=(0,D.useMemo)((()=>(0,l.p)({globalTool:"TiktokReviewInsights",location:"TiktokReviewInsights"})),[]);return(0,D.useEffect)((()=>{a&&m()}),[a,m]),(0,E.jsx)(o.ModalWindowRoot,{name:e,isDraggable:!0,isResizable:!0,resizeVisible:!0,headerPadding:"16px 8px 16px 16px",headerContent:(0,E.jsx)(zu,{}),rightContent:p?(0,E.jsx)($c,{children:(0,E.jsx)(Wa,{used:g,total:h,onUpgrade:m})}):void 0,onSync:d,children:(0,E.jsx)(Gc,{children:(0,E.jsxs)(Fc,{children:[(0,E.jsx)(Fa,{}),n&&(0,E.jsx)(Cc,{data:n}),(0,E.jsx)(jc,{}),(0,E.jsx)(zc,{children:(0,E.jsx)(Au,{isLimitReached:a,used:g,total:h,onUpgrade:m})})]})})})},$c=B.default.div`
  position: absolute;
  left: 50%;
  top: 50%;
  transform: translate(-50%, -50%);
  pointer-events: auto;
`,Gc=B.default.div`
  display: flex;
  flex-direction: column;
  overflow: hidden;
  min-height: 540px;
  height: 100%;
`,Fc=B.default.div`
  display: flex;
  flex-direction: column;
  align-items: stretch;
  justify-content: flex-start;
  flex: 1;
  border-top: 1px solid ${({theme:e})=>e.systemColors.grey200};
  overflow: hidden;
`,zc=B.default.div`
  flex: 1;
  display: flex;
  overflow: hidden;
  padding: ${({theme:e})=>e.spacing.sp24};
`;var Uc=Object.defineProperty,_c=Object.getOwnPropertySymbols,Wc=Object.prototype.hasOwnProperty,Vc=Object.prototype.propertyIsEnumerable,Hc=(e,t,n)=>t in e?Uc(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n,Zc=(e,t)=>{for(var n in t||(t={}))Wc.call(t,n)&&Hc(e,n,t[n]);if(_c)for(var n of _c(t))Vc.call(t,n)&&Hc(e,n,t[n]);return e};const Kc=e=>(0,E.jsx)(o.RootElement,{id:e.name,platform:"tiktok",useShadow:!0,fontsStyle:!0,globalStyle:!0,faStyle:!0,children:(0,E.jsx)(Bc,Zc({},e))});var Qc=(e,t,n)=>new Promise(((r,o)=>{var a=e=>{try{l(n.next(e))}catch(e){o(e)}},i=e=>{try{l(n.throw(e))}catch(e){o(e)}},l=e=>e.done?r(e.value):Promise.resolve(e.value).then(a,i);l((n=n.apply(e,t)).next())}));(0,a.isTiktokPage)()&&Qc(void 0,null,(function*(){w&&console.log("Start Extension Tiktok!"),yield(0,r.initIncognitoForContentScript)(),window.addEventListener(b.e$,(()=>{if(console.log("Token expired event fired"),I)return console.log("Token reset already in progress, queuing follow-up reset"),void(T=!0);P().then()})),(0,o.onGetStore)("sync",["extensionEnabled","isUserLogged","settings","activeExperiments"],(e=>Qc(this,[e],(function*({extensionEnabled:e,isUserLogged:t,settings:n,activeExperiments:r}){e?(w&&console.log("Extension Tiktok is enabled!"),(0,o.addModalState)("h10-tiktok-review-insights","tiktok",Kc),ga(!!t,n||{}).then()):w&&console.log("Extension Tiktok is disabled!")}))))})).then()},84638:(e,t,n)=>{"use strict";n.d(t,{U:()=>a,d:()=>i});var r=n(27044),o=n(32756);const a=e=>{return t=void 0,n=null,r=function*(){const t=yield(0,o.O)(e),n=((null==t?void 0:t.reviews)||[]).map((({count:e,timestamp:t})=>[1e3*t,e])),r=((null==t?void 0:t.rating)||[]).map((({rating:e,timestamp:t})=>[1e3*t,e]));if(0===n.length&&0===r.length)throw new Error("noData");return{review:n,rating:r}},new Promise(((e,o)=>{var a=e=>{try{l(r.next(e))}catch(e){o(e)}},i=e=>{try{l(r.throw(e))}catch(e){o(e)}},l=t=>t.done?e(t.value):Promise.resolve(t.value).then(a,i);l((r=r.apply(t,n)).next())}));var t,n,r},i=(e,t)=>{const n=Date.now();let o=[],a=[];e&&(0===t?(o=e.review,a=e.rating):(o=e.review.filter((e=>new Date(e[0]).getTime()>n-t*r.dayMilliseconds)),a=e.rating.filter((e=>new Date(e[0]).getTime()>n-t*r.dayMilliseconds))));return{reviewData:o,ratingData:a,reviewDataPercentage:(0,r.getAverageCalculations)(o),ratingDataPercentage:(0,r.getAverageCalculations)(a)}}},37483:(e,t,n)=>{"use strict";n.d(t,{d:()=>d,w:()=>p});var r=n(27044);var o=Object.defineProperty,a=Object.defineProperties,i=Object.getOwnPropertyDescriptors,l=Object.getOwnPropertySymbols,s=Object.prototype.hasOwnProperty,u=Object.prototype.propertyIsEnumerable,c=(e,t,n)=>t in e?o(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n;const d=e=>{return t=void 0,n=[e],o=function*({asin:e,marketplace:t,days:n}){const o=yield(0,r.getSalesGraph)({asin:e,marketplace:t,days:n}),d=({x:e,y:t})=>[1e3*e,t],p=e=>e?{sales:e.sales.map(d),moving:e.movAvg.map(d),trend:e.trend.map(d)}:{sales:[],moving:[],trend:[]},g=(h=((e,t)=>{for(var n in t||(t={}))s.call(t,n)&&c(e,n,t[n]);if(l)for(var n of l(t))u.call(t,n)&&c(e,n,t[n]);return e})({},p(o)),m={byChild:p(o.byChild)},a(h,i(m)));var h,m;return g},new Promise(((e,r)=>{var a=e=>{try{l(o.next(e))}catch(e){r(e)}},i=e=>{try{l(o.throw(e))}catch(e){r(e)}},l=t=>t.done?e(t.value):Promise.resolve(t.value).then(a,i);l((o=o.apply(t,n)).next())}));var t,n,o},p=(e,t,n)=>{const o=Date.now(),a=n?e:null==e?void 0:e.byChild;let i=[],l=[];a&&(0===t?(i=a.sales,l=a.moving):(i=a.sales.filter((e=>new Date(e[0]).getTime()>o-t*r.dayMilliseconds)),l=a.moving.filter((e=>new Date(e[0]).getTime()>o-t*r.dayMilliseconds))));const s=i.length?(e=>{const t=[];if(e.length){const n=(e=>{const t=e.length;let n=0,r=0,o=0,a=0;e.forEach(((e,t)=>{n+=t,r+=e[1],o+=t*e[1],a+=t*t}));const i=n/t,l=r/t,s=a/t-i*i,u=0===s?0:(o/t-i*l)/s;return{a:l-u*i,b:u}})(e);e.forEach(((r,o)=>{if(0===o||o===e.length-1){let e=n.a+n.b*(o+1);e<0&&(e=0),t.push([r[0],e])}}))}return t})(i):[];return{salesData:i,movingData:l,trendData:s,metrics:{sales:(0,r.getAverageCalculations)(i)}}}},69897:(e,t,n)=>{"use strict";n.d(t,{_:()=>o});var r=n(18780);const o=()=>new Promise((e=>{var t,n;const o=(0,r.uc)("accountId");null==(n=null==(t=null==chrome?void 0:chrome.storage)?void 0:t.local)||n.get([o],(t=>{var n,r;(null==t?void 0:t[o])?e(t[o]):null==(r=null==(n=null==chrome?void 0:chrome.storage)?void 0:n.sync)||r.get([o],(t=>{(null==t?void 0:t[o])?e(t[o]):e("")}))}))}))},31541:(e,t,n)=>{"use strict";n.d(t,{k7:()=>c,fx:()=>y,Fd:()=>g,Kr:()=>h,yw:()=>d,ZF:()=>u,Sm:()=>p,HA:()=>l,NH:()=>s,ie:()=>a,tQ:()=>Ue,JD:()=>gt,DO:()=>Ct,SC:()=>Nt});var r=n(27044),o=n(82274);const a="https://members.helium10.com/user/signin",i=`${r.membersLink}/api/v1`,l=`${`${i}/keywords/search-volume`}/chart`,s=`${i}/walmart/search-volume/chart`,u=`${r.researchLink}/api/v1/amazon/cpc-history`,c=`${r.membersLink}/keyword-manager/upload/add-folder`,d=`${r.membersLink}/keyword-manager/upload/get-upload-folders`,p=`${r.membersLink}/keyword-manager/upload/pull-keywords`,g=(e,t)=>e.includes("walmart")?`https://${e}/ip/${t}`:`https://${e}/${t?`dp/${t}?psc=1`:""}`,h=(e,t)=>t.includes("walmart")?`https://${t}/search?q=${encodeURIComponent(e).replace(/ /g,"+")}`:`https://${t}/s?k=${encodeURIComponent(e).replace(/ /g,"+")}`;(0,o.isAmazonProductPage)()||(0,o.isAmazonSearchPage)()||(0,o.isAmazonBestSellersPage)();var m=n(67749),f=(e,t,n)=>new Promise(((r,o)=>{var a=e=>{try{l(n.next(e))}catch(e){o(e)}},i=e=>{try{l(n.throw(e))}catch(e){o(e)}},l=e=>e.done?r(e.value):Promise.resolve(e.value).then(a,i);l((n=n.apply(e,t)).next())}));const v="text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8",y=e=>f(void 0,null,(function*(){return(e=>{var t;if("undefined"==typeof window||!(null==(t=window.location)?void 0:t.origin))return!1;try{return new URL(e).origin===window.location.origin}catch(e){return!1}})(e)?(e=>f(void 0,null,(function*(){try{const t=yield fetch(e,{method:"GET",credentials:"include",headers:{Accept:v}});return t.ok?t.text():(console.error(`[getPageFromContentScript] HTTP ${t.status} ${t.statusText} for ${e}`),null)}catch(t){return console.error(`[getPageFromContentScript] Failed to fetch ${e}`,t),null}})))(e):(e=>(0,m.H)({url:e,type:r.backgroundMessageTypes.BACKGROUND_REQUEST,withCredentials:!0,headers:{Accept:v,"Content-Type":void 0}}))(e)}));var k=n(85893),x=n(92565),w=n(17625),b=n(13025),C=n(66339),M=n(80690),L=n(40957),S=n(89107),j=n(67294),I=n(12788),T=n(76248);var P=n(94482),E=n(98641),D=n(52332),O=n(70533),A=n(50161);const R=e=>{return t=void 0,n=null,o=function*(){return(0,m.H)({url:yield(0,r.updateLinkWithAccountId)(c),type:r.backgroundMessageTypes.BACKGROUND_REQUEST,method:"POST",params:e})},new Promise(((e,r)=>{var a=e=>{try{l(o.next(e))}catch(e){r(e)}},i=e=>{try{l(o.throw(e))}catch(e){r(e)}},l=t=>t.done?e(t.value):Promise.resolve(t.value).then(a,i);l((o=o.apply(t,n)).next())}));var t,n,o};const N=e=>{return t=void 0,n=null,o=function*(){return(0,m.H)({url:yield(0,r.updateLinkWithAccountId)(d),method:"POST",params:e})},new Promise(((e,r)=>{var a=e=>{try{l(o.next(e))}catch(e){r(e)}},i=e=>{try{l(o.throw(e))}catch(e){r(e)}},l=t=>t.done?e(t.value):Promise.resolve(t.value).then(a,i);l((o=o.apply(t,n)).next())}));var t,n,o};const B=e=>{return t=void 0,n=null,o=function*(){return(0,m.H)({url:yield(0,r.updateLinkWithAccountId)(p),type:r.backgroundMessageTypes.BACKGROUND_REQUEST,method:"POST",params:e})},new Promise(((e,r)=>{var a=e=>{try{l(o.next(e))}catch(e){r(e)}},i=e=>{try{l(o.throw(e))}catch(e){r(e)}},l=t=>t.done?e(t.value):Promise.resolve(t.value).then(a,i);l((o=o.apply(t,n)).next())}));var t,n,o};var $=Object.defineProperty,G=Object.getOwnPropertySymbols,F=Object.prototype.hasOwnProperty,z=Object.prototype.propertyIsEnumerable,U=(e,t,n)=>t in e?$(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n,_=(e,t)=>{for(var n in t||(t={}))F.call(t,n)&&U(e,n,t[n]);if(G)for(var n of G(t))z.call(t,n)&&U(e,n,t[n]);return e},W=(e,t,n)=>new Promise(((r,o)=>{var a=e=>{try{l(n.next(e))}catch(e){o(e)}},i=e=>{try{l(n.throw(e))}catch(e){o(e)}},l=e=>e.done?r(e.value):Promise.resolve(e.value).then(a,i);l((n=n.apply(e,t)).next())}));const V=(0,A.F)(((e,t)=>_(_(_(_(_(_({},(0,E.a)(e,t,"data",null)),(0,E.a)(e,t,"searchValue","")),(0,E.a)(e,t,"folderName","")),(0,D.o)(e,t,"getUploadFolders",(e=>N({marketplace:(0,r.getCurrentMarketplace)()})),{fulfilledReaction:e=>t().data.set(e),rejectedReaction:(e,t)=>{const n=(0,r.getTranslation)();O.c.error(t||(null==n?void 0:n.t("xRayKeywords.error.serverError")))}})),(0,D.o)(e,t,"pullKeywords",(e=>W(void 0,[e],(function*({targetFolderId:e,onSuccess:t,keywords:n}){const r=yield B({targetFolderId:e,keywords:n});return null==t||t(),r}))),{fulfilledReaction:(e,{targetFolderName:t,keywords:n})=>{const o=(0,r.getTranslation)();"success"===(null==e?void 0:e.status)?O.c.success(null==o?void 0:o.t("xRayKeywords.addToMyList.addedToMyFolderSuccess",{count:n.length,targetFolderName:t}),{closeButton:void 0}):"All phrases already exist in this folder"===(null==e?void 0:e.message)?O.c.warning(null==o?void 0:o.t("xRayKeywords.createNewFolder.allPhrasesAlreadyExist")):O.c.warning(null==e?void 0:e.message)},rejectedReaction:(e,t)=>{const n=(0,r.getTranslation)();O.c.error(t||(null==n?void 0:n.t("xRayKeywords.error.serverError")))},initialStatus:"init"})),(0,D.o)(e,t,"createNewFolder",(e=>W(void 0,[e],(function*({data:e}){return yield R(e)}))),{fulfilledReaction:()=>{t().getUploadFolders.action(),t().folderName.set("")},rejectedReaction:(e,n,o)=>{const a=(0,r.getTranslation)();t().folderName.set(""),400===(null==o?void 0:o.responseStatus)?O.c.error(null==a?void 0:a.t("xRayKeywords.createNewFolder.folderAlreadyExist")):O.c.error(null==a?void 0:a.t("xRayKeywords.error.serverError"))}})))),H=({data:e})=>{var t;return(null==(t=e.value)?void 0:t.folders)?Object.values(e.value.folders):[]},Z=({data:e,searchValue:t})=>{var n;const r=(null==(n=e.value)?void 0:n.folders)?Object.values(e.value.folders):[];return r?null==r?void 0:r.filter((e=>e.name.toLowerCase().includes(t.value.toLowerCase()))):[]},K=({onChangeVisibility:e,title:t,isOpen:n})=>{const{t:o}=(0,r.useTranslation)(),[a,i]=V((e=>[e.folderName.value,e.createNewFolder.action]),T.X);return(0,k.jsx)(k.Fragment,{children:n?(0,k.jsxs)(Q,{children:[(0,k.jsx)(P.R,{buttonType:"cancel",size:"100",onClick:()=>e(!1),type:"button",children:o("buttons.cancel")}),(0,k.jsx)(P.R,{buttonType:"secondary",size:"100",onClick:()=>{e(!1),i({data:{name:a.trim(),marketplaceId:(0,r.getMarketDataFromDomain)().id}})},type:"submit",disabled:!a.trim().length,children:o("buttons.confirm")})]}):(0,k.jsx)(P.R,{buttonType:"secondary",size:"100",onClick:()=>e(!0),type:"button",children:t||o("xRayKeywords.createNewFolder.button")})})},Q=I.default.div`
  ${P.R} + ${P.R} {
    margin-left: ${({theme:e})=>e.spacing.sp8};
  }
`;var Y=n(48372);const X=({pullKeywords:e,children:t,visibleItemsCount:n=5})=>{const r=V(Z);return(0,k.jsxs)(q,{visibleItemsCount:n,children:[r.length>0&&r.map((t=>(0,k.jsx)(J,{pullKeywords:e,id:t.id,name:t.name,phrasesQty:t.phrasesQty},t.id))),t]})},q=I.default.div`
  overflow-y: auto;
  min-height: 20px;
  max-height: ${({visibleItemsCount:e})=>35*(e||5)}px;

  div {
    ${Y.pb}
  }
`,J=(0,j.memo)((({pullKeywords:e,id:t,name:n,phrasesQty:r})=>(0,k.jsxs)(ee,{"data-id":t,"data-name":n,onClick:e,isRow:!0,children:[(0,k.jsx)("span",{children:n}),(0,k.jsx)("span",{children:r})]}))),ee=I.default.div`
  display: flex;
  justify-content: space-between;
  align-items: center;

  & > {
    :first-child {
      max-width: 130px;
      min-width: 130px;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }
  }

  ${e=>e.isRow?I.css`
          ${S.nO};
          cursor: pointer;
          border-bottom: solid 1px #d9e0e8;

          &:hover {
            background-color: ${({theme:e})=>e.colors.defaultGrey};
          }
        `:I.css`
          ${S.zM};
          margin-top: 10px;
          background-color: #f2f5f7;
          border-top: solid 1px #d9e0e8;
        `}

  padding: ${e=>e.theme.spacing.sp8};
`;var te=n(19302);var ne=Object.defineProperty,re=Object.defineProperties,oe=Object.getOwnPropertyDescriptors,ae=Object.getOwnPropertySymbols,ie=Object.prototype.hasOwnProperty,le=Object.prototype.propertyIsEnumerable,se=(e,t,n)=>t in e?ne(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n;const ue=(0,I.default)(w.G)`
  position: relative;
  margin: 10px;
`,ce=I.default.div`
  display: flex;
  align-items: center;
  justify-content: space-between;
`,de=I.default.div`
  margin-top: ${e=>e.theme.spacing.sp8};
  position: relative;
`,pe=I.default.div`
  display: flex;
  align-items: center;
  justify-content: flex-end;
  margin-top: ${e=>e.theme.spacing.sp16};
`,ge=I.default.div`
  white-space: nowrap;
  ${S.nn};
  margin-right: ${e=>e.theme.spacing.sp16};
  color: ${e=>e.theme.systemColors.black};
`,he=I.default.div`
  position: relative !important;
`,me=I.default.div`
  box-sizing: border-box;
  min-height: ${({minHeight:e})=>e}px;
  position: absolute;
  top: 100%;
  left: 0;
  display: flex;
  flex-direction: column;
  padding: 16px;
  max-width: 270px;
  min-width: 270px;
  font-size: 14px;
  border-radius: 6px;
  box-shadow: 0 0 20px 0 rgb(43 57 72 / 16%);
  border: solid 1px #d9e0e8;
  background-color: #fff;
  overflow: hidden;
  z-index: 1000;
  margin: 8px 0 0;
  color: #212529;
  text-align: left;
`,fe=I.default.div`
  font-size: 27px;
  display: flex;
  align-items: center;

  & > svg {
    width: 100%;
  }
`,ve=I.default.div`
  padding: 8px;
  display: flex;
  flex-direction: column;
  align-items: center;
`,ye=I.default.img`
  width: 77px;
  height: 72px;
`,ke=I.default.div`
  ${S.nn};
  color: ${e=>e.theme.systemColors.black};
  margin-top: ${e=>e.theme.spacing.sp16};
`,xe=I.default.div`
  ${S.Qv};
  margin-top: ${e=>e.theme.spacing.sp4};
  margin-bottom: ${e=>e.theme.spacing.sp16};
  text-align: center;
`;Object.assign((e=>{var t=e,{parent:n,children:o,marketplace:a,visibleItemsCount:i,isOpen:l,setIsOpen:s}=t,u=((e,t)=>{var n={};for(var r in e)ie.call(e,r)&&t.indexOf(r)<0&&(n[r]=e[r]);if(null!=e&&ae)for(var r of ae(e))t.indexOf(r)<0&&le.call(e,r)&&(n[r]=e[r]);return n})(t,["parent","children","marketplace","visibleItemsCount","isOpen","setIsOpen"]);const{t:c}=(0,r.useTranslation)(),[d,p]=(0,j.useState)(!1),g=(0,j.useRef)(null),[h,m]=V((e=>[e.folderName.value,e.folderName.set]),T.X);Nt({ref:g,handler:()=>{s(!1)},parent:n});const f=e=>{setTimeout((()=>p(e)),1)},{foldersLength:v,handleSearchChangeDebounced:y,loading:w,status:S,error:I,pullKeywordLoading:P,pullKeywords:E}=(({isOpen:e,setIsOpen:t,selectedKeywords:n})=>{const o=V(H).length,[a,i,l,s,u,c,d]=V((e=>[e.getUploadFolders.action,"loading"===e.getUploadFolders.atom.status,e.getUploadFolders.atom.status,e.getUploadFolders.atom.error,e.pullKeywords.atom.status,e.pullKeywords.action,e.searchValue.set]),T.X),{accountId:p}=(0,r.useGetStore)("sync","accountId"),g=(0,j.useCallback)((e=>{const t=e.target.value||"";return d(t)}),[d]),h=(0,te.Z)(g,200);return(0,j.useEffect)((()=>{e&&a()}),[p,a,e]),{foldersLength:o,loading:i,status:l,error:s,handleSearchChangeDebounced:h,pullKeywordLoading:u,pullKeywords:(0,j.useCallback)((e=>{const r=Number(e.currentTarget.dataset.id),o=()=>{t(!1)};if(r)return c({targetFolderId:r,targetFolderName:e.currentTarget.dataset.name||"",keywords:n,onSuccess:o})}),[c,n,t])}})((D=((e,t)=>{for(var n in t||(t={}))ie.call(t,n)&&se(e,n,t[n]);if(ae)for(var n of ae(t))le.call(t,n)&&se(e,n,t[n]);return e})({},u),re(D,oe({isOpen:l,setIsOpen:s}))));var D;const O=(0,j.useMemo)((()=>b.i5[a]),[a]);return(0,k.jsxs)(he,{ref:g,children:[o,l&&(0,k.jsx)(me,{minHeight:260,children:(0,k.jsxs)(C.A1,{status:S,error:I,children:["loaded"===S&&(v>0||d)&&(0,k.jsxs)(k.Fragment,{children:[(0,k.jsxs)(ce,{children:[(0,k.jsx)(ge,{children:c("xRayKeywords.addToMyList.chooseFolder")}),(0,k.jsx)(fe,{children:O})]}),(0,k.jsxs)(de,{children:[(0,k.jsx)(C.A1,{status:P}),(0,k.jsx)(M._,{onChange:y,placeholder:c("xRayKeywords.addToMyList.searchPlaceholder"),size:"100"}),(0,k.jsxs)(ee,{children:[(0,k.jsx)("span",{children:c("xRayKeywords.addToMyList.folder")}),(0,k.jsx)("span",{children:c("xRayKeywords.addToMyList.countOfKeywords")})]}),!w||v>0?(0,k.jsx)(X,{pullKeywords:E,visibleItemsCount:i,children:d?(0,k.jsxs)(ee,{isRow:!0,children:[(0,k.jsx)(L.t,{size:"50",autoFocus:!0,onChange:e=>m(e.target.value),value:h}),(0,k.jsx)("span",{})]}):null}):(0,k.jsx)(ue,{icon:x.LM3,spin:!0})]}),(0,k.jsx)(pe,{children:(0,k.jsx)(K,{onChangeVisibility:f,isOpen:d})})]}),"loaded"===S&&0===v&&!d&&(0,k.jsxs)(ve,{children:[(0,k.jsx)(ye,{src:"data:image/svg+xml;base64,PD94bWwgdmVyc2lvbj0iMS4wIiBlbmNvZGluZz0iVVRGLTgiPz4KPHN2ZyB3aWR0aD0iNzdweCIgaGVpZ2h0PSI3MnB4IiB2aWV3Qm94PSIwIDAgNzcgNzIiIHZlcnNpb249IjEuMSIgeG1sbnM9Imh0dHA6Ly93d3cudzMub3JnLzIwMDAvc3ZnIiB4bWxuczp4bGluaz0iaHR0cDovL3d3dy53My5vcmcvMTk5OS94bGluayI+CiAgICA8dGl0bGU+MTVEM0ZCNjktQjhDRC00NzcwLUIxQ0UtNjU0Q0MwMjFFNTk3PC90aXRsZT4KICAgIDxkZWZzPgogICAgICAgIDxwb2x5Z29uIGlkPSJwYXRoLTEiIHBvaW50cz0iMCAwIDY0LjU1NDMyOTUgMCA2NC41NTQzMjk1IDY0Ljc3NzcgMCA2NC43Nzc3Ij48L3BvbHlnb24+CiAgICAgICAgPHBvbHlnb24gaWQ9InBhdGgtMyIgcG9pbnRzPSItNy4wODEwMDI1ZS0xNSAwIDMxLjg4NTgxNDYgMCAzMS44ODU4MTQ2IDMxLjk5NiAtNy4wODEwMDI1ZS0xNSAzMS45OTYiPjwvcG9seWdvbj4KICAgIDwvZGVmcz4KICAgIDxnIGlkPSJVSS0tLXY4IiBzdHJva2U9Im5vbmUiIHN0cm9rZS13aWR0aD0iMSIgZmlsbD0ibm9uZSIgZmlsbC1ydWxlPSJldmVub2RkIj4KICAgICAgICA8ZyBpZD0iTWFnbmV0LS0tQWRkLWZyb20tbXktbGlzdC1lbXB0eS1zdGF0ZSIgdHJhbnNmb3JtPSJ0cmFuc2xhdGUoLTE1MS4wMDAwMDAsIC0zOTIuMDAwMDAwKSI+CiAgICAgICAgICAgIDxnIGlkPSJHcm91cC00NyIgdHJhbnNmb3JtPSJ0cmFuc2xhdGUoNDguMDAwMDAwLCAzNjguMDAwMDAwKSI+CiAgICAgICAgICAgICAgICA8ZyBpZD0iR3JvdXAtMTciIHRyYW5zZm9ybT0idHJhbnNsYXRlKDEwMy4wMDAwMDAsIDI0LjAwMDAwMCkiPgogICAgICAgICAgICAgICAgICAgIDxnIGlkPSJHcm91cC0zIiB0cmFuc2Zvcm09InRyYW5zbGF0ZSgwLjAwMDAwMCwgNy4yMjIzMDApIj4KICAgICAgICAgICAgICAgICAgICAgICAgPG1hc2sgaWQ9Im1hc2stMiIgZmlsbD0id2hpdGUiPgogICAgICAgICAgICAgICAgICAgICAgICAgICAgPHVzZSB4bGluazpocmVmPSIjcGF0aC0xIj48L3VzZT4KICAgICAgICAgICAgICAgICAgICAgICAgPC9tYXNrPgogICAgICAgICAgICAgICAgICAgICAgICA8ZyBpZD0iQ2xpcC0yIj48L2c+CiAgICAgICAgICAgICAgICAgICAgICAgIDxwYXRoIGQ9Ik02NC41NTQzMjk1LDMyLjM4OSBDNjQuNTU0MzI5NSw1MC4yNzcgNTAuMTAzMTc2Niw2NC43NzggMzIuMjc2NjY2NSw2NC43NzggQzE0LjQ1MTE1MjksNjQuNzc4IDAsNTAuMjc3IDAsMzIuMzg5IEMwLDE0LjUwMSAxNC40NTExNTI5LDAgMzIuMjc2NjY2NSwwIEM1MC4xMDMxNzY2LDAgNjQuNTU0MzI5NSwxNC41MDEgNjQuNTU0MzI5NSwzMi4zODkiIGlkPSJGaWxsLTEiIGZpbGw9IiNFOEY2RkYiIG1hc2s9InVybCgjbWFzay0yKSI+PC9wYXRoPgogICAgICAgICAgICAgICAgICAgIDwvZz4KICAgICAgICAgICAgICAgICAgICA8cGF0aCBkPSJNMjkuOTI2MzczNCw0OS43OTk3IEwyOS45MjYzNzM0LDU3LjQ2NTcgQzI5LjkyNjM3MzQsNTcuNTU3NyAyOS45NDczMDEzLDU3LjY0NzcgMjkuOTg4MTYwMyw1Ny43Mjk3IEwzMS43NTEwNzk0LDYxLjI2NzcgQzMxLjk2NzMzMzUsNjEuNzAxNyAzMi41ODYxOTg4LDYxLjcwMTcgMzIuODAyNDUyOCw2MS4yNjc3IEwzNC41NjUzNzE5LDU3LjcyOTcgQzM0LjYwNjIzMSw1Ny42NDc3IDM0LjYyNzE1ODgsNTcuNTU3NyAzNC42MjcxNTg4LDU3LjQ2NTcgTDM0LjYyNzE1ODgsNDkuNzk5NyBMMjkuOTI2MzczNCw0OS43OTk3IFoiIGlkPSJGaWxsLTQiIGZpbGw9IiNBNkI3QzkiPjwvcGF0aD4KICAgICAgICAgICAgICAgICAgICA8ZyBpZD0iR3JvdXAtOCIgdHJhbnNmb3JtPSJ0cmFuc2xhdGUoNDUuMTE0MTg1LCAwLjAwMDAwMCkiPgogICAgICAgICAgICAgICAgICAgICAgICA8bWFzayBpZD0ibWFzay00IiBmaWxsPSJ3aGl0ZSI+CiAgICAgICAgICAgICAgICAgICAgICAgICAgICA8dXNlIHhsaW5rOmhyZWY9IiNwYXRoLTMiPjwvdXNlPgogICAgICAgICAgICAgICAgICAgICAgICA8L21hc2s+CiAgICAgICAgICAgICAgICAgICAgICAgIDxnIGlkPSJDbGlwLTciPjwvZz4KICAgICAgICAgICAgICAgICAgICAgICAgPHBhdGggZD0iTTMxLjg4NjAxNCwxNS45OTggQzMxLjg4NjAxNCwyNC44MzMgMjQuNzQ3NjM2NywzMS45OTYgMTUuOTQzMDA3LDMxLjk5NiBDNy4xMzczODA2NywzMS45OTYgLTcuMDgxMDAyNWUtMTUsMjQuODMzIC03LjA4MTAwMjVlLTE1LDE1Ljk5OCBDLTcuMDgxMDAyNWUtMTUsNy4xNjMgNy4xMzczODA2NywwIDE1Ljk0MzAwNywwIEMyNC43NDc2MzY3LDAgMzEuODg2MDE0LDcuMTYzIDMxLjg4NjAxNCwxNS45OTgiIGlkPSJGaWxsLTYiIGZpbGw9IiNBNkI3QzkiIG1hc2s9InVybCgjbWFzay00KSI+PC9wYXRoPgogICAgICAgICAgICAgICAgICAgIDwvZz4KICAgICAgICAgICAgICAgICAgICA8cGF0aCBkPSJNNjAuOTAwODMxNywxNy42OTYxIEw2MS42MDE0MTUxLDE3LjY5NjEgTDYxLjY0ODI1MzYsMTcuNDM0MSBDNjEuNjg5MTEyNiwxNy4wOTIxIDYxLjc5Mjc1NTEsMTYuNzkwMSA2MS45NjQxNjM5LDE2LjUyODEgQzYyLjEzNDU3NjEsMTYuMjY2MSA2Mi4zODk2OTYxLDE1Ljk0ODEgNjIuNzI5NTIzOSwxNS41NzQxIEM2Mi45NzU2NzQ4LDE1LjI5NjEgNjMuMTcxMDAxMSwxNS4wNjQxIDYzLjMxODQ5MjMsMTQuODc3MSBDNjMuNDYzOTkwNCwxNC42OTAxIDYzLjU4NjU2NzYsMTQuNDc0MSA2My42ODYyMjM5LDE0LjIyNzEgQzYzLjc4NDg4MzYsMTMuOTgxMSA2My44MzQ3MTE3LDEzLjcxNTEgNjMuODM0NzExNywxMy40MjgxIEM2My44MzQ3MTE3LDEyLjg1NjEgNjMuNjE2NDY0NSwxMi4zOTUxIDYzLjE4MDk2NjcsMTIuMDQ2MSBDNjIuNzQ1NDY4OSwxMS42OTYxIDYyLjE3NTQzNTEsMTEuNTIxMSA2MS40NzA4NjU0LDExLjUyMTEgQzYwLjk3MTU4NzYsMTEuNTIxMSA2MC40NzIzMDk4LDExLjYxMzEgNTkuOTc0MDI4NSwxMS43OTUxIEM1OS40NzQ3NTA3LDExLjk3ODEgNTkuMDU5MTg0MiwxMi4yMjUxIDU4LjcyNjMzMjMsMTIuNTM0MSBMNTkuMTY2ODEyOSwxMy40NDAxIEM1OS41MzA1NTgyLDEzLjE0NjEgNTkuOTA0MjY5MiwxMi45MjIxIDYwLjI4ODk0MjMsMTIuNzY3MSBDNjAuNjcyNjE4OSwxMi42MTIxIDYxLjA0MzM0MDEsMTIuNTM0MSA2MS4zOTkxMTI5LDEyLjUzNDEgQzYxLjc3OTc5OTgsMTIuNTM0MSA2Mi4wNzY3NzU0LDEyLjYyMjEgNjIuMjkwMDM5OCwxMi43OTcxIEM2Mi41MDQzMDA3LDEyLjk3MjEgNjIuNjExOTI5NSwxMy4yMTQxIDYyLjYxMTkyOTUsMTMuNTI0MSBDNjIuNjExOTI5NSwxMy43OTQxIDYyLjU1MjEzNTcsMTQuMDM2MSA2Mi40MzI1NDgyLDE0LjI1MTEgQzYyLjMxMzk1NzMsMTQuNDY2MSA2Mi4xMjc2MDAxLDE0LjczMjEgNjEuODc0NDczMiwxNS4wNTAxIEM2MS41NTc1NjY0LDE1LjQzOTEgNjEuMzE1NDAxNywxNS43OTcxIDYxLjE0ODk3NTcsMTYuMTIzMSBDNjAuOTgzNTQ2NCwxNi40NDgxIDYwLjkwMDgzMTcsMTYuODQ2MSA2MC45MDA4MzE3LDE3LjMxNTEgTDYwLjkwMDgzMTcsMTcuNjk2MSBaIE02MC41NTYwMjEsMjAuMDIxMSBMNjEuOTcwMTQzMiwyMC4wMjExIEw2MS45NzAxNDMyLDE4LjYwMjEgTDYwLjU1NjAyMSwxOC42MDIxIEw2MC41NTYwMjEsMjAuMDIxMSBaIiBpZD0iRmlsbC05IiBmaWxsPSIjRkZGRkZGIj48L3BhdGg+CiAgICAgICAgICAgICAgICAgICAgPHBhdGggZD0iTTYxLjA1NzA5MjcsMjMuMTgxNyBDNTcuMTA4NzEyLDIzLjE4MTcgNTMuODk2NzkxMSwxOS45NTk3IDUzLjg5Njc5MTEsMTUuOTk3NyBDNTMuODk2NzkxMSwxMi4wMzY3IDU3LjEwODcxMiw4LjgxMzcgNjEuMDU3MDkyNyw4LjgxMzcgQzY1LjAwNTQ3MzMsOC44MTM3IDY4LjIxNjM5NzcsMTIuMDM2NyA2OC4yMTYzOTc3LDE1Ljk5NzcgQzY4LjIxNjM5NzcsMTkuOTU5NyA2NS4wMDU0NzMzLDIzLjE4MTcgNjEuMDU3MDkyNywyMy4xODE3IE02MS4wNTcwOTI3LDcuNzY5NyBDNTYuNTM1Njg4Niw3Ljc2OTcgNTIuODU2Mzc5OCwxMS40NjA3IDUyLjg1NjM3OTgsMTUuOTk3NyBDNTIuODU2Mzc5OCwyMC41MzU3IDU2LjUzNTY4ODYsMjQuMjI2NyA2MS4wNTcwOTI3LDI0LjIyNjcgQzY1LjU3ODQ5NjgsMjQuMjI2NyA2OS4yNTc4MDU1LDIwLjUzNTcgNjkuMjU3ODA1NSwxNS45OTc3IEM2OS4yNTc4MDU1LDExLjQ2MDcgNjUuNTc4NDk2OCw3Ljc2OTcgNjEuMDU3MDkyNyw3Ljc2OTciIGlkPSJGaWxsLTExIiBmaWxsPSIjRkZGRkZGIj48L3BhdGg+CiAgICAgICAgICAgICAgICAgICAgPHBhdGggZD0iTTI1LjM4NzMzMDIsMzAuOTkxOCBMMjQuNDg4NDMwOCwzOS43MDg4IEMyMC45MjU3MTk5LDQxLjM3MTggMTguMTczMjE0Miw0NC4zNTI4IDE4LjE3MzIxNDIsNDguMDkxOCBDMTguMTczMjE0Miw0OS4wNjg4IDE4Ljk2MjQ5MTcsNDkuODYwOCAxOS45MzYxMzMzLDQ5Ljg2MDggTDQ0LjYxNzk5NjksNDkuODYwOCBDNDUuNTkxNjM4NSw0OS44NjA4IDQ2LjM4MDkxNiw0OS4wNjg4IDQ2LjM4MDkxNiw0OC4wOTE4IEM0Ni4zNzk5MTk0LDQ0LjMxOTggNDMuNTkwNTQwOSw0MS4zNTM4IDQwLjA2NTY5OTQsMzkuNzA4OCBMMzkuMTY2OCwzMC45OTE4IEwyNS4zODczMzAyLDMwLjk5MTggWiIgaWQ9IkZpbGwtMTMiIGZpbGw9IiM1Q0IzRkYiPjwvcGF0aD4KICAgICAgICAgICAgICAgICAgICA8cGF0aCBkPSJNMzkuMTY2NjAwNywzMS4wMjg3IEw0Mi4yNjY5MDY2LDMxLjAyODcgQzQzLjI0MDU0ODIsMzEuMDI4NyA0NC4wMjk4MjU3LDMwLjIzNjcgNDQuMDI5ODI1NywyOS4yNTk3IEw0NC4wMjk4MjU3LDI1LjcyMTcgQzQ0LjAyOTgyNTcsMjQuNzQ0NyA0My4yNDA1NDgyLDIzLjk1MjcgNDIuMjY2OTA2NiwyMy45NTI3IEwyMi4yODY4MjQ5LDIzLjk1MjcgQzIxLjMxMzE4MzQsMjMuOTUyNyAyMC41MjM5MDU5LDI0Ljc0NDcgMjAuNTIzOTA1OSwyNS43MjE3IEwyMC41MjM5MDU5LDI5LjI1OTcgQzIwLjUyMzkwNTksMzAuMjM2NyAyMS4zMTMxODM0LDMxLjAyODcgMjIuMjg2ODI0OSwzMS4wMjg3IEwyNS4zODcxMzA5LDMxLjAyODcgTDM5LjE2NjYwMDcsMzEuMDI4NyBaIiBpZD0iRmlsbC0xNSIgZmlsbD0iI0E1REJGRiI+PC9wYXRoPgogICAgICAgICAgICAgICAgPC9nPgogICAgICAgICAgICA8L2c+CiAgICAgICAgPC9nPgogICAgPC9nPgo8L3N2Zz4="}),(0,k.jsx)(ke,{children:c("xRayKeywords.addToMyList.emptyTitle")}),(0,k.jsx)(xe,{children:c("xRayKeywords.addToMyList.emptyText")}),(0,k.jsx)(K,{onChangeVisibility:f,title:c("xRayKeywords.addToMyList.emptyButton"),isOpen:d})]})]})})]})}),{Wrapper:he,Content:me});var we=n(51398),be=n(99293),Ce=n(70282);const Me=({asin:e,marketplace:t})=>{var n,o;const a=(0,Ce.eV)(),[i,l]=(0,Ce.OW)(a,2),{status:s,bsrGraphData:u,error:c,refreshGraphData:d,errorStatusCode:p}=Ue({days:i,asin:e,marketplace:t});return(0,k.jsxs)("div",{children:[(0,k.jsx)(Y.P1,{range:a,active:i,changeDays:l}),(0,k.jsx)(Le,{status:s,errorMsg:null!=c?c:"",refresh:d,errorStatusCode:p,children:(0,k.jsx)(Y.jR,{asin:e,bsr:u.bsr,price:u.price,listPrice:u.listPrice,height:400,minDate:i?Date.now()-i*r.dayMilliseconds:null==(o=null==(n=u.bsr)?void 0:n[0])?void 0:o[0],days:i})})]})},Le=(0,I.default)(be.Z)`
  height: 400px;
  padding: 0 60px;

`,Se=({asin:e,marketplace:t})=>{const n=(0,Ce.Ki)(),[r,o]=(0,Ce.OW)(n),{status:a,reviewData:i,ratingData:l}=gt(r,e,t);return(0,k.jsxs)("div",{children:[(0,k.jsx)(Y.P1,{range:n,active:r,changeDays:o}),"loading"===a?(0,k.jsx)(je,{children:(0,k.jsx)(Y.xZ,{})}):(0,k.jsx)(Y.Bk,{asin:e,days:r,reviews:i||[],rating:l||[],height:400})]})},je=I.default.div`
  height: 400px;
  display: flex;
  align-items: center;
  justify-content: center;
`;var Ie=n(15286);const Te=({asin:e,marketplace:t,isParentLevel:n})=>{const{t:o}=(0,r.useTranslation)(),a=(0,Ce.Ki)(),[i,l]=(0,Ce.OW)(a),s=t||(0,r.getCurrentMarketplaceTLDomain)(),u=Ie.xs.includes(s),{status:c,salesData:d,movingData:p,trendData:g,error:h}=Ct(i,e,t,n);return(0,k.jsxs)("div",{children:[(0,k.jsx)(Y.P1,{range:a,active:i,changeDays:l}),"loading"===c?(0,k.jsx)(Pe,{children:(0,k.jsx)(Y.xZ,{})}):(0,k.jsx)(Y.oP,{asin:e,sales:d||[],trend:g||[],average:p||[],days:i,height:400,isDecimalSupported:u,error:"limit"===h?o("errors.limit"):""})]})},Pe=I.default.div`
  height: 400px;
  display: flex;
  align-items: center;
  justify-content: center;
`,Ee=["xraySales","xrayBsr","xrayReview"],De=((0,j.memo)((({plan:e,asin:t,marketplace:n,title:o,defaultGraph:a,globalTool:i,location:l,hasValue:s,onOpen:u,children:c,align:d="flex-end",isParentLevel:p=!0})=>{const{t:g}=(0,r.useTranslation)(),[h,m]=(0,j.useState)(a),f=Ee.map((e=>({children:g(`uiComponents.graphs.${e}`),value:e})));return(0,k.jsx)(Re,{align:d,children:s?(0,k.jsx)(Y.KG,{status:"loaded",modalWidth:800,plan:e,location:l,globalTool:i,title:(0,k.jsx)(Ae,{children:o}),content:(0,k.jsx)(Re,{children:c}),onOpen:u,onHide:()=>m(a),children:(0,k.jsxs)("div",{children:[(0,k.jsx)(De,{children:(0,k.jsx)(we.Z,{buttons:f,onChange:e=>m(e),value:h})}),"xraySales"===h&&(0,k.jsx)(Te,{asin:t,marketplace:n,isParentLevel:p}),"xrayBsr"===h&&(0,k.jsx)(Me,{asin:t,marketplace:n}),"xrayReview"===h&&(0,k.jsx)(Se,{asin:t,marketplace:n})]})}):(0,k.jsx)(Oe,{children:g("common.notAvailable")})})})),I.default.div`
  padding-top: 24px;
  margin: 0 80px;
`),Oe=I.default.span`
  color: #999;
`,Ae=I.default.div`
  color: ${({theme:e})=>e.colors.white};
  white-space: nowrap;
  max-width: 660px;
  text-overflow: ellipsis;
  overflow: hidden;
`,Re=I.default.div`
  display: flex;
  align-items: center;
  justify-content: ${({align:e})=>e||"flex-end"};
`,Ne=I.default.div`
  width: 10px;
  height: 10px;
  border-radius: 5px;
  margin-right: 8px;
`,Be=I.default.div``;I.default.div`
  display: flex;
  align-items: center;
  min-width: 160px;
  ${Ne} {
    background-color: ${({disabled:e,color:t})=>e?"#a6b7c9":t};
  }
  ${Be} {
    color: ${({disabled:e})=>e?"#a6b7c9":"initial"};
  }
`;Object.defineProperty,Object.defineProperties,Object.getOwnPropertyDescriptors,Object.getOwnPropertySymbols,Object.prototype.hasOwnProperty,Object.prototype.propertyIsEnumerable;I.default.div`
  display: flex;
  align-items: center;
  justify-content: center;
  position: relative;
`,I.default.div`
  position: absolute;
  right: 10px;
  width: 170px;
`,I.default.div``,(0,I.default)(P.R)`
  margin-left: auto;
  display: flex;
  gap: ${({theme:e})=>e.spacing.sp8};
`;n(25252);var $e=n(67139),Ge=n(69119),Fe=n(83894),ze=n(24257);const Ue=({days:e,asin:t,marketplace:n})=>{const o=(0,j.useMemo)((()=>n||(0,r.getCurrentMarketplace)()),[n]),{isGrayscale:a,isReady:i}=(0,$e.U)(),l=(0,j.useMemo)((()=>a?Ce.Cd:e>0&&e<=30?30:0),[e,a]),{t:s}=(0,r.useTranslation)(),{data:u,getGraphData:c,error:d,status:p,errorStatusCode:g}=(0,r.graphsStore)((0,r.bsrGraphsSelector)(t,o,l,a),T.X),h=(0,j.useMemo)((()=>d?s(d):""),[s,d]),m=(0,j.useMemo)((()=>(0,Y.MP)(u.bsr||[],u.price||[],u.listPrice||[])),[u]),f=(0,j.useMemo)((()=>_e(m.bsr,e)),[e,m.bsr]),v=(0,j.useMemo)((()=>_e(m.price,e)),[e,m.price]),y=(0,j.useMemo)((()=>_e(m.listPrice,e)),[e,m.listPrice]),k=(0,j.useCallback)((()=>{c(!0)}),[c]);return(0,j.useEffect)((()=>{i&&(u.days===l&&(0,r.isReadyStatus)(p)||c())}),[l,i]),{status:p,bsrGraphData:m,bsrDataPercentage:f,priceDataPercentage:v,listPriceDataPercentage:y,error:h,errorStatusCode:g,refreshGraphData:k}},_e=(e,t)=>{if(0===t)return(0,r.getAverageCalculations)(e);const n=Date.now(),o=(0,Ge.Z)(n-t*r.dayMilliseconds).getTime(),a={start:o,end:(0,Fe.Z)(n).getTime()},i=e.filter((e=>(0,ze.Z)(e[0],a))),l=e.findLastIndex((e=>e[0]<o));return l>=0&&i.unshift([o,e[l][1]]),(0,r.getAverageCalculations)(i)};var We=n(69897);const Ve=e=>{return t=void 0,n=[e],o=function*({marketplaceId:e,periodFrom:t,periodTo:n,phraseId:o}){const a=yield(0,We._)();return(0,m.H)({url:u,type:r.backgroundMessageTypes.BACKGROUND_REQUEST,params:{accountId:a,marketplaceId:e,periodFrom:t,periodTo:n,"phraseIds[]":o},useSessionAccountId:!0})},new Promise(((e,r)=>{var a=e=>{try{l(o.next(e))}catch(e){r(e)}},i=e=>{try{l(o.throw(e))}catch(e){r(e)}},l=t=>t.done?e(t.value):Promise.resolve(t.value).then(a,i);l((o=o.apply(t,n)).next())}));var t,n,o};var He=Object.defineProperty,Ze=Object.defineProperties,Ke=Object.getOwnPropertyDescriptors,Qe=Object.getOwnPropertySymbols,Ye=Object.prototype.hasOwnProperty,Xe=Object.prototype.propertyIsEnumerable,qe=(e,t,n)=>t in e?He(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n,Je=(e,t)=>{for(var n in t||(t={}))Ye.call(t,n)&&qe(e,n,t[n]);if(Qe)for(var n of Qe(t))Xe.call(t,n)&&qe(e,n,t[n]);return e};const et={ppcBid:[]},tt=((0,A.F)(((e,t)=>Je(Je({},(0,E.a)(e,t,"data",{ppc:{}})),(0,D.o)(e,t,"getPpcData",(e=>{return n=void 0,o=[e],a=function*({periodFrom:e,periodTo:n,phraseId:o,days:a}){var i;const l=t().data.value,s=(0,r.getMarketDataFromDomain)(),u=`${o}_${a}`;if(l.ppc[u])return;tt(t,{cacheKey:u});const c=yield Ve({marketplaceId:s.id,periodFrom:e,periodTo:n,phraseId:o});if(!c)throw new Error("request");const d={ppcBid:null!=(i=null==c?void 0:c.data.reverse().map((e=>[new Date(e.date).getTime(),(0,r.priceFromApi)(e.cpc,s.currency)])))?i:[]};tt(t,{cacheKey:u,data:d})},new Promise(((e,t)=>{var r=e=>{try{l(a.next(e))}catch(e){t(e)}},i=e=>{try{l(a.throw(e))}catch(e){t(e)}},l=t=>t.done?e(t.value):Promise.resolve(t.value).then(r,i);l((a=a.apply(n,o)).next())}));var n,o,a}),{rejectedReaction:({phraseId:e,days:n})=>{tt(t,{cacheKey:`${e}_${n}`})}})))),(e,{cacheKey:t,data:n,error:r})=>{const o=e().data.value.ppc,a=void 0!==r?"error":void 0!==n?"loaded":"loading",i=null!=n?n:et;var l,s;e().data.set({ppc:(l=Je({},o),s={[t]:{data:{content:i,status:a,error:r}}},Ze(l,Ke(s)))})});Object.defineProperty,Object.getOwnPropertySymbols,Object.prototype.hasOwnProperty,Object.prototype.propertyIsEnumerable;var nt=n(33746),rt=n(84638),ot=n(34802),at=Object.defineProperty,it=Object.defineProperties,lt=Object.getOwnPropertyDescriptors,st=Object.getOwnPropertySymbols,ut=Object.prototype.hasOwnProperty,ct=Object.prototype.propertyIsEnumerable,dt=(e,t,n)=>t in e?at(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n,pt=(e,t)=>{for(var n in t||(t={}))ut.call(t,n)&&dt(e,n,t[n]);if(st)for(var n of st(t))ct.call(t,n)&&dt(e,n,t[n]);return e};const gt=(e,t,n)=>{var o;const a=(0,j.useMemo)((()=>n||(0,r.getCurrentMarketplace)()),[n]),{isGrayscale:i,isReady:l}=(0,$e.U)(),s=(0,j.useMemo)((()=>(0,Ce._c)(e,i,Ce.Cd)),[e,i]),{data:u,status:c,error:d}=(0,nt.a)((p=pt({},ot.o.global.reviewGraphMetrics({asin:t,marketplace:a,days:s})),g={staleTime:36e5,persister:(0,ot.V)(),enabled:l},it(p,lt(g))));var p,g;const h=(0,j.useMemo)((()=>(0,rt.d)(u,e)),[u,e]),m=(0,j.useMemo)((()=>(0,r.transReactQueryStatus)(c)),[c]);return pt({status:m,error:null!=(o=null==d?void 0:d.message)?o:void 0},h)};var ht=n(37483),mt=Object.defineProperty,ft=Object.defineProperties,vt=Object.getOwnPropertyDescriptors,yt=Object.getOwnPropertySymbols,kt=Object.prototype.hasOwnProperty,xt=Object.prototype.propertyIsEnumerable,wt=(e,t,n)=>t in e?mt(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n,bt=(e,t)=>{for(var n in t||(t={}))kt.call(t,n)&&wt(e,n,t[n]);if(yt)for(var n of yt(t))xt.call(t,n)&&wt(e,n,t[n]);return e};const Ct=(e,t,n,o=!0)=>{var a;const i=(0,j.useMemo)((()=>n||(0,r.getCurrentMarketplace)()),[n]),{isGrayscale:l,isReady:s}=(0,$e.U)(),u=(0,j.useMemo)((()=>(0,Ce._c)(e,l,Ce.Cd)),[e,l]),{data:c,status:d,error:p}=(0,nt.a)((g=bt({},ot.o.global.salesGraphMetrics({asin:t,marketplace:i,days:u})),h={staleTime:36e5,persister:(0,ot.V)(),enabled:s},ft(g,vt(h))));var g,h;const m=(0,j.useMemo)((()=>(0,ht.w)(c,e,o)),[c,e,o]),f=(0,j.useMemo)((()=>(0,r.transReactQueryStatus)(d)),[d]);return bt({status:f,error:null!=(a=null==p?void 0:p.message)?a:void 0},m)};const Mt=e=>{return t=void 0,n=[e],o=function*({platform:e,keyword:t,marketplace:n,days:o}){const a=yield(0,We._)(),i={AMAZON:l,WALMART:s,GOOGLE:(0,r.searchGoogleGraphDataLink)(a,t)};return yield(0,m.H)({url:i[e],type:r.backgroundMessageTypes.BACKGROUND_REQUEST,params:{keyword:"GOOGLE"!==e?t:void 0,marketplace:"AMAZON"===e?n:void 0,moduleId:"GOOGLE"!==e?"xray":void 0,accountId:a,days:o},useSessionAccountId:"GOOGLE"===e})},new Promise(((e,r)=>{var a=e=>{try{l(o.next(e))}catch(e){r(e)}},i=e=>{try{l(o.throw(e))}catch(e){r(e)}},l=t=>t.done?e(t.value):Promise.resolve(t.value).then(a,i);l((o=o.apply(t,n)).next())}));var t,n,o};var Lt=Object.defineProperty,St=Object.defineProperties,jt=Object.getOwnPropertyDescriptors,It=Object.getOwnPropertySymbols,Tt=Object.prototype.hasOwnProperty,Pt=Object.prototype.propertyIsEnumerable,Et=(e,t,n)=>t in e?Lt(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n,Dt=(e,t)=>{for(var n in t||(t={}))Tt.call(t,n)&&Et(e,n,t[n]);if(It)for(var n of It(t))Pt.call(t,n)&&Et(e,n,t[n]);return e},Ot=(e,t)=>St(e,jt(t));const At={search:[]},Rt=((0,A.F)(((e,t)=>Dt(Dt({},(0,E.a)(e,t,"data",{search:{}})),(0,D.o)(e,t,"getSearchVolume",(e=>{return n=void 0,o=[e],a=function*({keyword:e,platform:n,marketplaceStore:o,days:a=30}){var i,l,s,u;const c=t().data.value,d=o||(0,r.getMarketDataFromDomain)().store,p="GOOGLE"===n||0===a?0:a;if(null==(s=null==(l=null==(i=c.search)?void 0:i[e])?void 0:l[n])?void 0:s[p])return;Rt(t,{keyword:e,platform:n,days:p});const g=yield Mt({platform:n,keyword:e,marketplace:d,days:"GOOGLE"!==n&&a>0?a:void 0});if(!g)throw new Error("request");const h="GOOGLE"===n?{results:{history:g.data,keyword:e,marketplace:d}}:g,m={search:null==(u=null==h?void 0:h.results.history)?void 0:u.map((({timestamp:e,exact:t})=>[1e3*e,t]))};Rt(t,{keyword:e,platform:n,days:p,data:m})},new Promise(((e,t)=>{var r=e=>{try{l(a.next(e))}catch(e){t(e)}},i=e=>{try{l(a.throw(e))}catch(e){t(e)}},l=t=>t.done?e(t.value):Promise.resolve(t.value).then(r,i);l((a=a.apply(n,o)).next())}));var n,o,a}),{rejectedReaction:({keyword:e,platform:n,days:r=30})=>{Rt(t,{keyword:e,platform:n,days:"GOOGLE"===n||0===r?0:r,error:null})}})))),(e,{keyword:t,platform:n,days:r,data:o,error:a})=>{var i,l,s;const u=e().data.value.search,c=void 0!==a?"error":void 0!==o?"loaded":"loading",d=null!=o?o:At;e().data.set({search:Ot(Dt({},u),{[t]:Ot(Dt({},null!=(i=u[t])?i:{}),{[n]:Ot(Dt({},null!=(s=null==(l=u[t])?void 0:l[n])?s:{}),{[r]:{data:{content:d,status:c,error:a}}})})})})});Object.defineProperty,Object.defineProperties,Object.getOwnPropertyDescriptors,Object.getOwnPropertySymbols,Object.prototype.hasOwnProperty,Object.prototype.propertyIsEnumerable;const Nt=({ref:e,handler:t,parent:n,isActive:r=!0})=>{(0,j.useEffect)((()=>{const o=n=>{r&&e.current&&!e.current.contains(n.target)&&t(n)},a=n instanceof HTMLElement?n:document;return a.addEventListener("click",o),()=>{a.removeEventListener("click",o)}}),[e,t,n,r])}},34802:(e,t,n)=>{"use strict";n.d(t,{V:()=>g,o:()=>p});var r=n(10987),o=n(90526),a=n(27044),i=n(82274),l=n(41267),s=n(4976),u=n(84638),c=n(37483);const d=(0,r.no)("global",{salesGraphMetrics:e=>({queryKey:[e],queryFn:()=>(0,c.d)(e)}),reviewGraphMetrics:e=>({queryKey:[e],queryFn:()=>(0,u.U)(e)}),orderHistoryProduct:e=>({queryKey:[e],queryFn:()=>(0,l.l)(e)}),orderHistorySale:e=>({queryKey:[e],queryFn:()=>(0,s.x)(e)}),creatorConnectionAsins:e=>({queryKey:[e],queryFn:()=>(0,i.getProductInfoByCampaignId)(e)}),getCreatorStudioInformation:()=>({queryKey:["creatorStudioInformation"],queryFn:()=>(0,i.getCreatorStudioInformation)()})}),p=(0,r.kR)(d),g=()=>(0,o.c)({prefix:"h10",storage:a.chromeStorageLocalPersister,maxAge:72e5})},41267:(e,t,n)=>{"use strict";n.d(t,{l:()=>i});var r=n(27044),o=n(82274);n(67294),n(34802);const a=(e,t)=>{var n,r;let o="none",a=!1,i=!1;if((null==(n=null==e?void 0:e.upperCarouselVideosInfo)?void 0:n.length)>0)for(const n of e.upperCarouselVideosInfo)for(const e of t)if("VIDEO"===e.contentType&&e.id===n.videoId){a=!0;break}if((null==(r=null==e?void 0:e.lowerCarouselVideosInfo)?void 0:r.length)>0)for(const n of e.lowerCarouselVideosInfo)for(const e of t)if("VIDEO"===e.contentType&&e.id===n.videoId){i=!0;break}return a&&i?o="both":a?o="top":i&&(o="bottom"),o};Object.defineProperty,Object.defineProperties,Object.getOwnPropertyDescriptors,Object.getOwnPropertySymbols,Object.prototype.hasOwnProperty,Object.prototype.propertyIsEnumerable;const i=e=>{return t=void 0,n=null,i=function*(){var t,n,i,l,s,u,c,d,p,g,h,m,f,v,y,k,x;const w=yield(0,o.getProductDetails)(e),b=r.queryClient.getQueryData(["orderHistoryVideos"]);return{asin:e.asin,totalVideos:(null==(n=null==(t=w.product)?void 0:t.carouselVideo)?void 0:n.influencerVideos)||-1,top:(null==(l=null==(i=w.product)?void 0:i.carouselVideo)?void 0:l.upperCarouselInfluencerVideos)||-1,bottom:(null==(u=null==(s=w.product)?void 0:s.carouselVideo)?void 0:u.lowerCarouselInfluencerVideos)||-1,influencerCommission:100*(0,r.getCommissionRate)(null==(c=w.product)?void 0:c.category,"influencer"),affiliateCommission:100*(0,r.getCommissionRate)(null==(d=w.product)?void 0:d.category,"affiliate"),bsr:(null==(p=w.product)?void 0:p.bsr)||-1,price:(null==(g=w.product)?void 0:g.price)||-1,reviewsRating:(null==(h=w.product)?void 0:h.reviewsRating)||-1,reviewsNumber:null==(m=w.product)?void 0:m.reviewsNumber,placement:a(null==(f=w.product)?void 0:f.carouselVideo,(null==b?void 0:b.videosDetails)||[]),title:(null==(v=w.product)?void 0:v.title)||"",bsrList:null==(y=w.product)?void 0:y.bsrList,carouselVideo:null==(k=w.product)?void 0:k.carouselVideo,category:null==(x=w.product)?void 0:x.category}},new Promise(((e,r)=>{var o=e=>{try{l(i.next(e))}catch(e){r(e)}},a=e=>{try{l(i.throw(e))}catch(e){r(e)}},l=t=>t.done?e(t.value):Promise.resolve(t.value).then(o,a);l((i=i.apply(t,n)).next())}));var t,n,i}},4976:(e,t,n)=>{"use strict";n.d(t,{x:()=>a});var r=n(27044),o=n(82274);n(34802),Object.defineProperty,Object.defineProperties,Object.getOwnPropertyDescriptors,Object.getOwnPropertySymbols,Object.prototype.hasOwnProperty,Object.prototype.propertyIsEnumerable;const a=e=>{return t=void 0,n=null,a=function*(){var t,n,a,i;const l=yield(0,o.getSaleDetails)(e.asin);return{sale:l.sale?null==(t=(0,r.get30DaySalesByParent)(l.sale))?void 0:t.estMonthlySales:-1,revenue:(0,r.get30DayRevenueByParent)(l)||-1,salesData:l.sale,productTitle:null==(a=null==(n=l.sale)?void 0:n.productData)?void 0:a.title,salesHistory:null==(i=l.sale)?void 0:i.salesHistory}},new Promise(((e,r)=>{var o=e=>{try{l(a.next(e))}catch(e){r(e)}},i=e=>{try{l(a.throw(e))}catch(e){r(e)}},l=t=>t.done?e(t.value):Promise.resolve(t.value).then(o,i);l((a=a.apply(t,n)).next())}));var t,n,a}},29444:(e,t,n)=>{"use strict";n.d(t,{n:()=>Mt,p:()=>re});var r=n(782),o=n(50161),a=n(98641),i=n(30976),l=Object.defineProperty,s=Object.getOwnPropertySymbols,u=Object.prototype.hasOwnProperty,c=Object.prototype.propertyIsEnumerable,d=(e,t,n)=>t in e?l(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n,p=(e,t)=>{for(var n in t||(t={}))u.call(t,n)&&d(e,n,t[n]);if(s)for(var n of s(t))c.call(t,n)&&d(e,n,t[n]);return e};var g=Object.defineProperty,h=Object.getOwnPropertySymbols,m=Object.prototype.hasOwnProperty,f=Object.prototype.propertyIsEnumerable,v=(e,t,n)=>t in e?g(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n,y=(e,t)=>{for(var n in t||(t={}))m.call(t,n)&&v(e,n,t[n]);if(h)for(var n of h(t))f.call(t,n)&&v(e,n,t[n]);return e};var k=Object.defineProperty,x=Object.getOwnPropertySymbols,w=Object.prototype.hasOwnProperty,b=Object.prototype.propertyIsEnumerable,C=(e,t,n)=>t in e?k(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n,M=(e,t)=>{for(var n in t||(t={}))w.call(t,n)&&C(e,n,t[n]);if(x)for(var n of x(t))b.call(t,n)&&C(e,n,t[n]);return e};var L=Object.defineProperty,S=Object.getOwnPropertySymbols,j=Object.prototype.hasOwnProperty,I=Object.prototype.propertyIsEnumerable,T=(e,t,n)=>t in e?L(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n,P=(e,t)=>{for(var n in t||(t={}))j.call(t,n)&&T(e,n,t[n]);if(S)for(var n of S(t))I.call(t,n)&&T(e,n,t[n]);return e};var E=Object.defineProperty,D=Object.getOwnPropertySymbols,O=Object.prototype.hasOwnProperty,A=Object.prototype.propertyIsEnumerable,R=(e,t,n)=>t in e?E(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n,N=(e,t)=>{for(var n in t||(t={}))O.call(t,n)&&R(e,n,t[n]);if(D)for(var n of D(t))A.call(t,n)&&R(e,n,t[n]);return e};var B=Object.defineProperty,$=Object.getOwnPropertySymbols,G=Object.prototype.hasOwnProperty,F=Object.prototype.propertyIsEnumerable,z=(e,t,n)=>t in e?B(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n,U=(e,t)=>{for(var n in t||(t={}))G.call(t,n)&&z(e,n,t[n]);if($)for(var n of $(t))F.call(t,n)&&z(e,n,t[n]);return e};const _=(0,o.F)()((0,r.XR)(((...e)=>U(U(U(U(U({},((e,t)=>p(p({},(0,a.a)(e,t,"isDiamondActive",!1)),(0,i.f)(e,t,"diamondModal",null)))(...e)),((e,t)=>y(y({},(0,a.a)(e,t,"isPlatinumActive",!1)),(0,i.f)(e,t,"platinumModal",null)))(...e)),((e,t)=>M(M({},(0,a.a)(e,t,"isSubscriptionActive",!1)),(0,i.f)(e,t,"subscriptionModal",null)))(...e)),((e,t)=>P(P({},(0,a.a)(e,t,"isToolSpecificActive",!1)),(0,i.f)(e,t,"toolSpecificModal",null)))(...e)),((e,t)=>N(N({},(0,a.a)(e,t,"isTrackCompetitorsActive",!1)),(0,i.f)(e,t,"trackCompetitorsModal",null)))(...e)))));var W=n(92616);const V={toolTitle:"uiComponents.upgrade.upsellModal.productSummary.title",features:["uiComponents.upgrade.upsellModal.productSummary.features.productPageMetrics","uiComponents.upgrade.upsellModal.productSummary.features.xray","uiComponents.upgrade.upsellModal.productSummary.features.keywords","uiComponents.upgrade.upsellModal.productSummary.features.searchExpander","uiComponents.upgrade.upsellModal.productSummary.features.blackBox","uiComponents.upgrade.upsellModal.productSummary.features.magnet","uiComponents.upgrade.upsellModal.productSummary.features.cerebro"]},H={toolTitle:"uiComponents.upgrade.upsellModal.searchResultsPage.title",toolSubtitle:"uiComponents.upgrade.upsellModal.searchResultsPage.subtitle",features:["uiComponents.upgrade.upsellModal.searchResultsPage.features.keywordToolbar","uiComponents.upgrade.upsellModal.searchResultsPage.features.xrayResearch","uiComponents.upgrade.upsellModal.searchResultsPage.features.searchExpander","uiComponents.upgrade.upsellModal.searchResultsPage.features.xrayKeywords","uiComponents.upgrade.upsellModal.searchResultsPage.features.graphs","uiComponents.upgrade.upsellModal.searchResultsPage.features.exports","uiComponents.upgrade.upsellModal.searchResultsPage.features.researchTools"]},Z={toolTitle:"uiComponents.upgrade.titleWalmart",features:["uiComponents.upgrade.upsellModal.xrayWalmart.chromeExtension","uiComponents.upgrade.upsellModal.xrayWalmart.magnet","uiComponents.upgrade.upsellModal.xrayWalmart.cerebro","uiComponents.upgrade.upsellModal.xrayWalmart.profits","uiComponents.upgrade.upsellModal.xrayWalmart.sorting","uiComponents.upgrade.upsellModal.xrayWalmart.more"]},K={toolTitle:"uiComponents.upgrade.upsellModal.xrayKeyword.title",features:["uiComponents.upgrade.upsellModal.xrayKeyword.features.access","uiComponents.upgrade.upsellModal.xrayKeyword.features.sales","uiComponents.upgrade.upsellModal.xrayKeyword.features.keywordTracker","uiComponents.upgrade.upsellModal.xrayKeyword.features.cerebro","uiComponents.upgrade.upsellModal.xrayKeyword.features.listingAnalyzer","uiComponents.upgrade.upsellModal.xrayKeyword.features.marketTracker","uiComponents.upgrade.upsellModal.xrayKeyword.features.multiUser"]};var Q=Object.defineProperty,Y=Object.defineProperties,X=Object.getOwnPropertyDescriptors,q=Object.getOwnPropertySymbols,J=Object.prototype.hasOwnProperty,ee=Object.prototype.propertyIsEnumerable,te=(e,t,n)=>t in e?Q(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n,ne=(e,t)=>{for(var n in t||(t={}))J.call(t,n)&&te(e,n,t[n]);if(q)for(var n of q(t))ee.call(t,n)&&te(e,n,t[n]);return e};const re=e=>{const{openToolSpecific:t,openSubscription:n,openTrackCompetitors:r,openDiamond:o,openPlatinum:a}=(0,W.p)(_.getState()),i=(l=ne({},e),s={type:e.type||"featureLocked"},Y(l,X(s)));var l,s;return{openPlatinum:()=>t(i),openPlatinumSERP:()=>t(ne(ne({},i),H)),openPlatinumProductSummary:()=>t(ne(ne({},i),V)),openTrackCompetitors:()=>r(ne({},i)),openDiamondXrayKeywords:()=>o(ne(ne({},i),K)),openXrayUpsell:()=>{"ce_XrayWalmart"===i.globalTool?o(ne(ne({},i),Z)):t(i)},openOldSubscription:()=>n(i),openToolSpecific:()=>t(i)}};var oe=n(85893),ae=n(27044),ie=n(67294),le=n(12788),se=n(2825),ue=n(17625),ce=n(66270),de=n(14439);const pe=({container:e,containerRef:t,upsellCTAUpdate:n,canApplyCoupon:r})=>{const{t:o}=(0,ae.useTranslation)(),{data:a,props:i}=_((e=>({diamondModal:t})=>({data:t.atom.data,props:{container:e,isOpen:t.atom.isOpen,onClose:t.close,onClickOutside:()=>{}}}))(e||(null==t?void 0:t.current)||void 0)),l=(0,ie.useMemo)((()=>({title:o((null==a?void 0:a.toolTitle)||"uiComponents.upgrade.upsellModal.title"),subtitle:r?(0,oe.jsx)(fe,{dangerouslySetInnerHTML:{__html:o((null==a?void 0:a.toolSubtitle)||"uiComponents.upgrade.upsellModal.productSummary.subtitle")}}):void 0,plans:[{id:ce.N.Free,label:(0,oe.jsx)(oe.Fragment,{children:o(`plans.${ce.N.Free}`)})},{id:ce.N.ScaleYourBusiness,label:(0,oe.jsx)(me,{children:o(`plans.${ce.N.ScaleYourBusiness}`)}),isBest:!0,discount:r?20:void 0,price:(0,oe.jsxs)(ve,{canApplyCoupon:r,children:[(0,oe.jsx)("span",{children:"$279/mo"}),(0,oe.jsx)("span",{children:`$${(279*.8).toFixed(2)}/mo`})]})}],features:((null==a?void 0:a.features)||ge).map((e=>({title:(0,oe.jsx)(oe.Fragment,{children:o(e)}),subtitle:null,values:{[ce.N.Free]:(0,oe.jsx)(ue.G,{icon:se.Kl4}),[ce.N.ScaleYourBusiness]:(0,oe.jsx)(ue.G,{icon:se.LEp})}}))),footer:{label:null,labelWithCoupon:null,acceptButton:o(n?"uiComponents.upgrade.upsellModal.upgradeToDiamond":"uiComponents.upgrade.upsellModal.acceptButton"),declineButton:o("uiComponents.upgrade.upsellModal.declineButton")}})),[o,n,a,r]),s=(0,ie.useCallback)((()=>{const e=n?(0,ae.getUpgradeWithCouponPlanLinkV2)("diamond"):ae.upgradeWithCouponPlanLinkV1;(0,ae.updateLinkWithAccountId)(e).then((e=>window.open(e)))}),[n]);return(0,ie.useEffect)((()=>{a&&i.isOpen&&(0,ae.sendSegmentEventMessage)({name:ae.segmentEventType.upsellModalFired,properties:{tool:a.globalTool,location:a.location}}).then()}),[i.isOpen]),(0,oe.jsxs)(oe.Fragment,{children:[(0,oe.jsx)(he,{}),(0,oe.jsx)(de.n,{data:l,modalProps:i,onAccept:s,onDecline:i.onClose})]})},ge=["uiComponents.upgrade.upsellModal.features.xraySales","uiComponents.upgrade.upsellModal.features.searchVolume","uiComponents.upgrade.upsellModal.features.graphs","uiComponents.upgrade.upsellModal.features.xrayFilters","uiComponents.upgrade.upsellModal.features.exports","uiComponents.upgrade.upsellModal.features.salesEstimator","uiComponents.upgrade.upsellModal.features.researchTools","uiComponents.upgrade.upsellModal.features.freedomTicket"],he=le.createGlobalStyle`
  tr:first-child th {
    padding-top: 40px;
  }
`,me=le.default.div`
  font-size: 16px;
  white-space: nowrap;
`,fe=le.default.div``,ve=le.default.div`
  & > * {
    display: block;
  }

  & > *:first-child {
    text-decoration: ${({canApplyCoupon:e})=>e?"line-through":"none"};
    color: ${({canApplyCoupon:e,theme:t})=>e?t.colors.grey600:t.colors.grey800};
  }

  & > *:last-child {
    display: ${({canApplyCoupon:e})=>e?"block":"none"};
    color: ${({theme:e})=>e.colors.grey800};
  }
`,ye=({container:e,containerRef:t,upsellCTAUpdate:n,canApplyCoupon:r})=>{const{t:o}=(0,ae.useTranslation)(),{data:a,props:i}=_((e=>({platinumModal:t})=>({data:t.atom.data,props:{container:e,isOpen:t.atom.isOpen,onClose:t.close,onClickOutside:()=>{}}}))(e||(null==t?void 0:t.current)||void 0)),l=(0,ie.useMemo)((()=>({title:o((null==a?void 0:a.toolTitle)||"uiComponents.upgrade.upsellModal.title"),subtitle:r?(0,oe.jsx)(be,{dangerouslySetInnerHTML:{__html:o((null==a?void 0:a.toolSubtitle)||"uiComponents.upgrade.upsellModal.productSummary.subtitle")}}):void 0,plans:[{id:ce.N.Free,label:(0,oe.jsx)(oe.Fragment,{children:o(`plans.${ce.N.Free}`)})},{id:ce.N.StartYourBusiness,label:(0,oe.jsx)(we,{children:o(`plans.${ce.N.StartYourBusiness}`)}),isBest:!0,discount:r?20:void 0,price:(0,oe.jsxs)(Ce,{canApplyCoupon:!!r,children:[(0,oe.jsx)("span",{children:"$99/mo"}),(0,oe.jsx)("span",{children:`$${79.2.toFixed(2)}/mo`})]})}],features:((null==a?void 0:a.features)||xe).map((e=>({title:(0,oe.jsx)(oe.Fragment,{children:o(e)}),subtitle:null,values:{[ce.N.Free]:(0,oe.jsx)(ue.G,{icon:se.Kl4}),[ce.N.StartYourBusiness]:(0,oe.jsx)(ue.G,{icon:se.LEp})}}))),footer:{label:null,labelWithCoupon:null,acceptButton:o(n?"uiComponents.upgrade.upsellModal.upgradeToPlatinum":"uiComponents.upgrade.upsellModal.acceptButton"),declineButton:o("uiComponents.upgrade.upsellModal.declineButton")}})),[o,n,a,r]),s=(0,ie.useCallback)((()=>{const e=n?(0,ae.getUpgradeWithCouponPlanLinkV2)("platinum"):ae.upgradeWithCouponPlanLinkV1;(0,ae.updateLinkWithAccountId)(e).then((e=>window.open(e)))}),[n]);return(0,ie.useEffect)((()=>{a&&i.isOpen&&(0,ae.sendSegmentEventMessage)({name:ae.segmentEventType.upsellModalFired,properties:{tool:a.globalTool,location:a.location}}).then()}),[i.isOpen]),(0,oe.jsxs)(oe.Fragment,{children:[(0,oe.jsx)(ke,{}),(0,oe.jsx)(de.n,{data:l,modalProps:i,onAccept:s,onDecline:i.onClose})]})},ke=le.createGlobalStyle`
  tr:first-child th {
    padding-top: 40px;
  }
`,xe=["uiComponents.upgrade.upsellModal.features.xraySales","uiComponents.upgrade.upsellModal.features.searchVolume","uiComponents.upgrade.upsellModal.features.graphs","uiComponents.upgrade.upsellModal.features.xrayFilters","uiComponents.upgrade.upsellModal.features.exports","uiComponents.upgrade.upsellModal.features.salesEstimator","uiComponents.upgrade.upsellModal.features.researchTools","uiComponents.upgrade.upsellModal.features.freedomTicket"],we=le.default.div`
  font-size: 16px;
  white-space: nowrap;
`,be=le.default.div``,Ce=le.default.div`
  & > * {
    display: block;
  }

  & > *:first-child {
    text-decoration: ${({canApplyCoupon:e})=>e?"line-through":"none"};
    color: ${({canApplyCoupon:e,theme:t})=>e?t.colors.grey600:t.colors.grey800};
  }

  & > *:last-child {
    display: ${({canApplyCoupon:e})=>e?"block":"none"};
    color: ${({theme:e})=>e.colors.grey800};
  }
`;var Me=n(26419);const Le=({container:e,containerRef:t})=>{const{t:n}=(0,ae.useTranslation)(),{data:r,props:o}=_((e=>({subscriptionModal:t})=>({data:t.atom.data,props:{container:e,isOpen:t.atom.isOpen,onClose:t.close,onClickOutside:()=>{}}}))(e||(null==t?void 0:t.current)||void 0)),a=(0,ie.useCallback)((()=>{(0,ae.updateLinkWithAccountId)(ae.upgradePlanLink).then((e=>window.open(e)))}),[]);return(0,ie.useEffect)((()=>{r&&o.isOpen&&(0,ae.sendSegmentEventMessage)({name:ae.segmentEventType.upsellModalFired,properties:{tool:r.globalTool,location:r.location}}).then()}),[o.isOpen]),(0,oe.jsx)(ae.Modal,{node:e||(null==t?void 0:t.current)||void 0,size:"small",visible:o.isOpen||!1,onHide:()=>{var e;return null==(e=null==o?void 0:o.onClose)?void 0:e.call(o)},onConfirm:a,dark:!0,children:(0,oe.jsxs)(Se,{children:[(0,oe.jsx)(je,{children:(0,oe.jsx)(De,{src:Me.Z,alt:"upgrade"})}),(0,oe.jsx)(Pe,{children:n("uiComponents.subscriptionRequired.title")}),(0,oe.jsx)(Ee,{children:n("uiComponents.subscriptionRequired.searchVolume")})]})})},Se=le.default.div`
  text-align: center;
  display: flex;
  flex-direction: column;
  padding: 40px 40px 0 40px;
`,je=le.default.div`
  width: 80px;
  height: 80px;
  display: flex;
  background-color: rgba(0, 129, 255, 0.15);
  border-radius: 50%;
  margin: auto;
`,Ie=le.default.span`
  font-size: ${({theme:e})=>e.fontSize.large};
  font-weight: bold;
  line-height: 1.39;
  letter-spacing: 0.82px;
  color: ${({theme:e})=>e.colors.darkBlue};
`,Te=le.default.span`
  font-size: ${({theme:e})=>e.fontSize.headerSmall};
  font-weight: 600;
  line-height: 1.56;
  letter-spacing: 0.73px;
`,Pe=(0,le.default)(Ie)`
  padding: 24px 0 8px 0;
`,Ee=(0,le.default)(Te)`
  color: ${({theme:e})=>e.colors.darkGrey};
`,De=le.default.img`
  margin: auto;
`;var Oe=n(3346),Ae=n(67749);const Re=e=>(0,Ae.H)({url:(0,ae.upsellDataLink)(e),withCredentials:!0});var Ne=Object.defineProperty,Be=Object.defineProperties,$e=Object.getOwnPropertyDescriptors,Ge=Object.getOwnPropertySymbols,Fe=Object.prototype.hasOwnProperty,ze=Object.prototype.propertyIsEnumerable,Ue=(e,t,n)=>t in e?Ne(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n;const _e=({container:e,containerRef:t,upsellCTAUpdate:n,canApplyCoupon:r})=>{const{data:o,props:a}=_((e=>({toolSpecificModal:t})=>({data:t.atom.data||{type:"featureLocked"},props:{container:e,isOpen:t.atom.isOpen,onClose:t.close,onClickOutside:()=>{}}}))(e||(null==t?void 0:t.current)||void 0)),{mode:i}=(0,ae.useGetStore)("sync","mode"),l=(0,ie.useCallback)((()=>{var e;const t=n?(0,ae.getUpgradeWithCouponPlanLinkV2)("influencer"===i?"starter":"platinum",o.globalTool):(0,ae.getUpgradeWithCouponPlanLinkV1)(o.globalTool);"ce_Xray"===o.globalTool&&"influencer"===i&&(0,ae.sendSegmentEventMessage)({name:ae.segmentEventType.upsellModalFired,properties:{tool:o.openFrom||o.globalTool,location:o.location}}).then(),null==(e=o.onCustomAccept)||e.call(o),(0,ae.updateLinkWithAccountId)(t).then((e=>{(null==a?void 0:a.onClose)&&a.onClose(),window.open(e)}))}),[o.globalTool,o.location,o.openFrom,o.onCustomAccept,n,i,null==a?void 0:a.onClose]),s=(0,ie.useMemo)((()=>{return e=((e,t)=>{for(var n in t||(t={}))Fe.call(t,n)&&Ue(e,n,t[n]);if(Ge)for(var n of Ge(t))ze.call(t,n)&&Ue(e,n,t[n]);return e})({},o),t={tool:o.globalTool,coupon:r?(0,ae.getCoupon)(o.globalTool):void 0},Be(e,$e(t));var e,t}),[o,r]);return(0,ie.useEffect)((()=>{o&&a.isOpen&&(0,ae.sendSegmentEventMessage)({name:ae.segmentEventType.upsellModalFired,properties:{tool:o.openFrom||o.globalTool,location:o.location,product_type:o.productType}}).then()}),[o,o.location,o.globalTool,o.openFrom,a.isOpen]),(0,oe.jsx)(Oe.P,{data:s,modalProps:a,onCustomAccept:l,onDecline:a.onClose,getCustomAction:Re})};var We=n(1203),Ve=n(63499),He=n(89107),Ze=n(86702),Ke=Object.defineProperty,Qe=Object.defineProperties,Ye=Object.getOwnPropertyDescriptors,Xe=Object.getOwnPropertySymbols,qe=Object.prototype.hasOwnProperty,Je=Object.prototype.propertyIsEnumerable,et=(e,t,n)=>t in e?Ke(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n,tt=(e,t)=>{for(var n in t||(t={}))qe.call(t,n)&&et(e,n,t[n]);if(Xe)for(var n of Xe(t))Je.call(t,n)&&et(e,n,t[n]);return e},nt=(e,t)=>Qe(e,Ye(t));const rt=({container:e,containerRef:t,upsellCTAUpdate:n})=>{const{t:r}=(0,ae.useTranslation)(),{data:o,props:a}=_((e=>({trackCompetitorsModal:t})=>({data:t.atom.data,props:{container:e,isOpen:t.atom.isOpen,onClose:t.close,onClickOutside:()=>{}}}))(e||(null==t?void 0:t.current)||void 0)),[i]=(0,ie.useState)(249),l=(0,ie.useMemo)((()=>({title:(0,oe.jsxs)(it,{children:[r("uiComponents.upgrade.upsellModal.trackCompetitors.mainTitle"),(0,oe.jsx)(lt,{children:r("uiComponents.upgrade.upsellModal.trackCompetitors.mainSubtitle")})]}),featuresTitle:(0,oe.jsx)(st,{children:r("uiComponents.upgrade.upsellModal.trackCompetitors.features")}),plans:[{id:ce.N.Free,label:(0,oe.jsx)(st,{children:r(`plans.${ce.N.Free}`)})},{id:ce.N.ScaleYourBusiness,label:(0,oe.jsxs)(ut,{children:[(0,oe.jsx)(Ve.m,{category:"dataBlue",defaultValue:r("uiComponents.upgrade.upsellModal.trackCompetitors.bestPrice")}),(0,oe.jsx)(at,{children:r(`plans.${ce.N.ScaleYourBusiness}`)})]}),isBest:!0,price:r("uiComponents.upgrade.upsellModal.trackCompetitors.perMonth",{price:(0,ae.formatPrice)({amount:i,options:{maximumFractionDigits:0},marketplaceDomain:We.id.com.domain})})}],features:[{title:r("uiComponents.upgrade.upsellModal.trackCompetitors.competitorTracking.title"),subtitle:r("uiComponents.upgrade.upsellModal.trackCompetitors.competitorTracking.subtitle")},{title:r("uiComponents.upgrade.upsellModal.trackCompetitors.competitorKeywordResearch.title"),subtitle:r("uiComponents.upgrade.upsellModal.trackCompetitors.competitorKeywordResearch.subtitle")},{title:r("uiComponents.upgrade.upsellModal.trackCompetitors.insightsDashboard.title"),subtitle:r("uiComponents.upgrade.upsellModal.trackCompetitors.insightsDashboard.subtitle")},{title:r("uiComponents.upgrade.upsellModal.trackCompetitors.advancedAnalytics.title"),subtitle:r("uiComponents.upgrade.upsellModal.trackCompetitors.advancedAnalytics.subtitle")}],footer:{label:null,labelWithCoupon:null,acceptButton:r("uiComponents.upgrade.upsellModal.upgradeToDiamond"),declineButton:r("uiComponents.upgrade.upsellModal.declineButton")}})),[i,r]),s=l.features.map((e=>nt(tt({},e),{values:{[ce.N.Free]:(0,oe.jsx)(gt,{children:(0,oe.jsx)(ht,{icon:se.g82})}),[ce.N.ScaleYourBusiness]:(0,oe.jsx)(ct,{children:(0,oe.jsx)(pt,{children:(0,oe.jsx)(dt,{icon:se.LEp})})})}}))),u=(0,ie.useCallback)((()=>{const e=n?(0,ae.getUpgradeWithCouponPlanLinkV2)("diamond"):ae.upgradeWithCouponPlanLinkV1;(0,ae.updateLinkWithAccountId)(e).then((e=>window.open(e)))}),[n]);return(0,ie.useEffect)((()=>{o&&a.isOpen&&(0,ae.sendSegmentEventMessage)({name:ae.segmentEventType.upsellModalFired,properties:{tool:o.globalTool,location:o.location}}).then()}),[a.isOpen]),(0,oe.jsxs)(oe.Fragment,{children:[(0,oe.jsx)(ot,{}),(0,oe.jsx)(de.n,{data:nt(tt({},l),{features:s}),modalProps:a,onAccept:u,onDecline:a.onClose})]})},ot=le.createGlobalStyle`
  tr:first-child th {
    padding-top: 40px;
  }

  tr > th:last-child {
    ::after {
      content: "";
      height: 0;
    }

    ::before {
      border-top: 2px solid rgb(10, 111, 232);
      border-radius: 6px 6px 0 0;
    }
  }

  td {
    position: relative;
  }
`,at=le.default.div`
  font-size: 16px;
  white-space: nowrap;
`,it=le.default.div`
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: ${({theme:e})=>e.spacing.sp4};
`,lt=le.default.div`
  ${He.Qv};
`,st=le.default.div`
  margin-top: ${({theme:e})=>e.spacing.sp8};
`,ut=(0,le.default)(st)`
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: ${({theme:e})=>e.spacing.sp8};

  & > div:first-child {
    width: fit-content;
    position: absolute;
    top: 16px;
  }
`,ct=le.default.div`
  ${He.Qv};

  position: absolute;
  top: 50%;
  bottom: 50%;
  min-width: 120px;
  display: flex;
  align-items: center;
  justify-content: center;

  ${Ze.BC.laptop} {
    min-width: 160px;
  }
`,dt=(0,le.default)(ue.G)`
  font-size: 20px;
`,pt=le.default.div`
  display: flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border-radius: 50%;
  background-color: ${e=>e.theme.systemColors.blue600};

  ${dt} {
    color: ${e=>e.theme.systemColors.white};
  }
`,gt=le.default.div`
  ${He.nO};

  color: ${({theme:e})=>e.systemColors.grey600};
  min-width: 100px;
  display: flex;
  align-items: center;
  justify-content: center;
  text-align: center;
  position: absolute;
  top: 50%;
  bottom: 50%;

  ${Ze.BC.laptop} {
    min-width: 160px;
  }
`,ht=(0,le.default)(ue.G)`
  font-size: 20px;
  color: ${e=>e.theme.systemColors.grey300};
`;var mt=Object.defineProperty,ft=Object.getOwnPropertySymbols,vt=Object.prototype.hasOwnProperty,yt=Object.prototype.propertyIsEnumerable,kt=(e,t,n)=>t in e?mt(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n,xt=(e,t)=>{for(var n in t||(t={}))vt.call(t,n)&&kt(e,n,t[n]);if(ft)for(var n of ft(t))yt.call(t,n)&&kt(e,n,t[n]);return e};const wt=e=>{var t=((e,t)=>{var n={};for(var r in e)vt.call(e,r)&&t.indexOf(r)<0&&(n[r]=e[r]);if(null!=e&&ft)for(var r of ft(e))t.indexOf(r)<0&&yt.call(e,r)&&(n[r]=e[r]);return n})(e,[]);const n=_(W.D),{login:r}=(0,ae.useGetCacheStore)("cache-sync","login"),o=(0,ie.useMemo)((()=>{var e;return null==(e=null==r?void 0:r.upsellInfo)?void 0:e.canApplyCoupon}),[r]);return(0,oe.jsxs)(oe.Fragment,{children:[n.isPlatinumActive?(0,oe.jsx)(ye,xt({canApplyCoupon:o},t)):(0,oe.jsx)(oe.Fragment,{}),n.isDiamondActive?(0,oe.jsx)(pe,xt({canApplyCoupon:o},t)):(0,oe.jsx)(oe.Fragment,{}),n.isSubscriptionActive?(0,oe.jsx)(Le,xt({},t)):(0,oe.jsx)(oe.Fragment,{}),n.isToolSpecificActive?(0,oe.jsx)(_e,xt({canApplyCoupon:o},t)):(0,oe.jsx)(oe.Fragment,{}),n.isTrackCompetitorsActive?(0,oe.jsx)(rt,xt({canApplyCoupon:o},t)):(0,oe.jsx)(oe.Fragment,{})]})},bt=()=>{const e=(0,ie.useRef)(null);return(0,oe.jsx)(Ct,{ref:e,children:(0,oe.jsx)(wt,{containerRef:e})})},Ct=le.default.div``,Mt=({platform:e})=>(0,oe.jsx)(ae.RootElement,{id:"h10-upsell-modal",platform:e,useShadow:!0,fontsStyle:!0,isolatedGlobalStyle:!0,faStyle:!0,children:(0,oe.jsx)(bt,{})})},92616:(e,t,n)=>{"use strict";n.d(t,{D:()=>o,p:()=>r});const r=({isPlatinumActive:e,isDiamondActive:t,isSubscriptionActive:n,isToolSpecificActive:r,isTrackCompetitorsActive:o,platinumModal:a,diamondModal:i,subscriptionModal:l,toolSpecificModal:s,trackCompetitorsModal:u})=>({openPlatinum:t=>{e.value||e.set(!0),setTimeout((()=>a.open(t)))},openDiamond:e=>{t.value||t.set(!0),setTimeout((()=>i.open(e)))},openSubscription:e=>{n.value||n.set(!0),setTimeout((()=>l.open(e)))},openToolSpecific:e=>{r.value||r.set(!0),setTimeout((()=>s.open(e)))},openTrackCompetitors:e=>{o.value||o.set(!0),setTimeout((()=>u.open(e)))}}),o=({isPlatinumActive:e,isDiamondActive:t,isSubscriptionActive:n,isToolSpecificActive:r,isTrackCompetitorsActive:o})=>({isPlatinumActive:e.value,isDiamondActive:t.value,isSubscriptionActive:n.value,isToolSpecificActive:r.value,isTrackCompetitorsActive:o.value})},15286:(e,t,n)=>{"use strict";n.d(t,{xs:()=>r});const r=["com","co.uk","de","fr","es","it","ca","com.mx","in","co.jp","sa"]},25252:(e,t,n)=>{"use strict";n(27044),n(31541)},96424:()=>{},75381:()=>{}},o={};function a(e){var t=o[e];if(void 0!==t)return t.exports;var n=o[e]={id:e,exports:{}};return r[e].call(n.exports,n,n.exports,a),n.exports}a.m=r,a.amdO={},e=[],a.O=(t,n,r,o)=>{if(!n){var i=1/0;for(c=0;c<e.length;c++){for(var[n,r,o]=e[c],l=!0,s=0;s<n.length;s++)(!1&o||i>=o)&&Object.keys(a.O).every((e=>a.O[e](n[s])))?n.splice(s--,1):(l=!1,o<i&&(i=o));if(l){e.splice(c--,1);var u=r();void 0!==u&&(t=u)}}return t}o=o||0;for(var c=e.length;c>0&&e[c-1][2]>o;c--)e[c]=e[c-1];e[c]=[n,r,o]},a.n=e=>{var t=e&&e.__esModule?()=>e.default:()=>e;return a.d(t,{a:t}),t},n=Object.getPrototypeOf?e=>Object.getPrototypeOf(e):e=>e.__proto__,a.t=function(e,r){if(1&r&&(e=this(e)),8&r)return e;if("object"==typeof e&&e){if(4&r&&e.__esModule)return e;if(16&r&&"function"==typeof e.then)return e}var o=Object.create(null);a.r(o);var i={};t=t||[null,n({}),n([]),n(n)];for(var l=2&r&&e;"object"==typeof l&&!~t.indexOf(l);l=n(l))Object.getOwnPropertyNames(l).forEach((t=>i[t]=()=>e[t]));return i.default=()=>e,a.d(o,i),o},a.d=(e,t)=>{for(var n in t)a.o(t,n)&&!a.o(e,n)&&Object.defineProperty(e,n,{enumerable:!0,get:t[n]})},a.e=()=>Promise.resolve(),a.g=function(){if("object"==typeof globalThis)return globalThis;try{return this}catch(e){if("object"==typeof window)return window}}(),a.o=(e,t)=>Object.prototype.hasOwnProperty.call(e,t),a.r=e=>{"undefined"!=typeof Symbol&&Symbol.toStringTag&&Object.defineProperty(e,Symbol.toStringTag,{value:"Module"}),Object.defineProperty(e,"__esModule",{value:!0})},a.j=568,(()=>{var e={568:0};a.O.j=t=>0===e[t];var t=(t,n)=>{var r,o,[i,l,s]=n,u=0;if(i.some((t=>0!==e[t]))){for(r in l)a.o(l,r)&&(a.m[r]=l[r]);if(s)var c=s(a)}for(t&&t(n);u<i.length;u++)o=i[u],a.o(e,o)&&e[o]&&e[o][0](),e[o]=0;return a.O(c)},n=globalThis.webpackChunkhelium10_main_tool=globalThis.webpackChunkhelium10_main_tool||[];n.forEach(t.bind(null,0)),n.push=t.bind(null,n.push.bind(n))})(),a.nc=void 0,a.O(void 0,[175,844,270,44,997,216],(()=>a(35666)));var i=a.O(void 0,[175,844,270,44,997,216],(()=>a(1013)));i=a.O(i)})();