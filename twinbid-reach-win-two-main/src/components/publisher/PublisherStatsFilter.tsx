import { useState } from "react";
import { ChevronDown } from "lucide-react";
import { Checkbox } from "@/components/ui/checkbox";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { useLanguage } from "@/contexts/LanguageContext";
import { publisherUiText as tr } from "@/lib/publisherUiText";

type Option = { value: string; label: string };
export function PublisherStatsFilter({ id, label, options, value, onChange, multiple = false, compact = false, allLabel }: {
  id: string; label: string; options: Option[]; value: string; onChange: (value: string) => void; multiple?: boolean; compact?: boolean; allLabel?: string;
}) {
  const { lang } = useLanguage();
  const [search, setSearch] = useState("");
  const [open, setOpen] = useState(false);
  const selected = multiple ? value.split(",").map(v => v.trim()).filter(Boolean) : value ? [value] : [];
  const toggle = (v: string) => { onChange(multiple ? (selected.includes(v) ? selected.filter(s => s !== v) : [...selected, v]).join(", ") : selected.includes(v) ? "" : v); if (!multiple) setOpen(false); };
  return <div className={`flex min-w-0 flex-col ${compact ? "gap-1" : "gap-2"}`}>
    <Label htmlFor={id} className={`${compact ? "text-xs" : "text-sm font-medium"} text-muted-foreground`}>{label}</Label>
    <Popover open={open} onOpenChange={next => {setOpen(next);setSearch("");}}><PopoverTrigger asChild><Button id={id} variant="outline" className={`${compact ? "h-8 sm:w-[220px]" : "sm:w-[260px]"} w-full min-w-0 justify-start truncate border-border bg-background text-left text-sm font-normal`}>
      <span className="min-w-0 flex-1 truncate">{selected.length === 0 ? (allLabel || tr(lang, "Все значения", "All values", "全部")) : multiple ? tr(lang, `Выбрано: ${selected.length}`, `Selected: ${selected.length}`, `已选：${selected.length}`) : options.find(o => o.value === value)?.label || value}</span><ChevronDown className="ml-2 h-4 w-4 shrink-0 text-muted-foreground" />
    </Button></PopoverTrigger><PopoverContent className="w-[min(300px,calc(100vw-1rem))] p-2" align="start">
      <Input value={search} onChange={e => setSearch(e.target.value)} aria-label={tr(lang, "Поиск", "Search", "搜索")} placeholder={tr(lang, "Поиск…", "Search…", "搜索…")} className="mb-2 h-8" />
      <div className="max-h-56 space-y-1 overflow-y-auto">
        <label className="mb-1 flex cursor-pointer items-center gap-2 rounded border-b border-border px-2 py-1.5 pb-2 text-sm font-medium"><Checkbox checked={selected.length === 0} onCheckedChange={() => {onChange("");if(!multiple)setOpen(false);}} />{(allLabel || tr(lang, "Все значения", "All values", "全部"))}</label>
        {options.slice().sort((a,b)=>a.label.localeCompare(b.label)).filter(o => `${o.label} ${o.value}`.toLowerCase().includes(search.toLowerCase())).map(o => <label key={o.value} className="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-sm hover:bg-muted/50"><Checkbox checked={selected.includes(o.value)} onCheckedChange={() => toggle(o.value)} /><span className="min-w-0 truncate" title={o.label}>{o.label}</span></label>)}
      </div>
    </PopoverContent></Popover>
  </div>;
}
