export type Prelander = { prelander_id:string; generation_id:string; vertical:string; geo:string; language:string; style:string; offer_url:string; visual_path:string; preview_url:string; created_at:string };
export type GenerateResponse = { generation_id:string; items:Prelander[] };
