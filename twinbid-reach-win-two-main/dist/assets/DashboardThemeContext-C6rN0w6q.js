import{c as h,r,j as l}from"./index-XxPwhwta.js";/**
 * @license lucide-react v0.462.0 - ISC
 *
 * This source code is licensed under the ISC license.
 * See the LICENSE file in the root directory of this source tree.
 */const b=h("User",[["path",{d:"M19 21v-2a4 4 0 0 0-4-4H9a4 4 0 0 0-4 4v2",key:"975kel"}],["circle",{cx:"12",cy:"7",r:"4",key:"17ys0d"}]]),a="twinbid-dashboard-theme",s="dark",n=r.createContext(null);function m(){if(typeof window>"u")return s;const e=window.localStorage.getItem(a);return e==="light"||e==="dark"?e:s}function g({children:e}){const[o,d]=r.useState(m),c=t=>{d(t),window.localStorage.setItem(a,t)};r.useEffect(()=>{const t=document.documentElement;return t.classList.toggle("dashboard-light",o==="light"),t.classList.toggle("dashboard-dark",o==="dark"),t.style.colorScheme=o,()=>{t.classList.remove("dashboard-light","dashboard-dark"),t.style.removeProperty("color-scheme")}},[o]);const i=r.useMemo(()=>({theme:o,setTheme:c}),[o]);return l.jsx(n.Provider,{value:i,children:e})}function T(){const e=r.useContext(n);if(!e)throw new Error("useDashboardTheme must be used inside DashboardThemeProvider");return e}export{g as D,b as U,T as u};
