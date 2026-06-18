import type { GenerateResponse, Prelander } from '../types/prelander';
export const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080';
async function read<T>(res: Response): Promise<T> { if (!res.ok) throw new Error((await res.json().catch(()=>({error:'Request failed'}))).error); return res.json(); }
export const generatePrelanders = (form: FormData) => read<GenerateResponse>(fetch(`${API_BASE_URL}/api/prelanders/generate`, { method:'POST', body: form }));
export const listPrelanders = () => read<Prelander[]>(fetch(`${API_BASE_URL}/api/prelanders`));
