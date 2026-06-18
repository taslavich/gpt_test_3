import { useEffect, useState } from 'react';
import { listPrelanders } from '../api/client';
import type { Prelander } from '../types/prelander';
import { PrelanderCard } from './PrelanderCard';
export function PrelandersList(){ const [items,setItems]=useState<Prelander[]>([]); const [error,setError]=useState(''); useEffect(()=>{listPrelanders().then(setItems).catch(e=>setError(e.message))},[]); if(error) return <p className="error">{error}</p>; return <div className="grid">{items.map(i=><PrelanderCard key={i.prelander_id} item={i}/>)}</div> }
