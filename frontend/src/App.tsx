import { useState } from 'react';
import { HomePage } from './pages/HomePage';
import { PrelandersPage } from './pages/PrelandersPage';
import './styles.css';
export default function App(){ const [page,setPage]=useState<'home'|'prelanders'>('home'); return <div><nav><b>AI Prelander Builder</b><button onClick={()=>setPage('home')}>Generate</button><button onClick={()=>setPage('prelanders')}>Prelanders</button></nav><main>{page==='home'?<HomePage/>:<PrelandersPage/>}</main></div> }
