import { useLanguage, LANGUAGE_OPTIONS, type Lang } from "@/contexts/LanguageContext";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Button } from "@/components/ui/button";
import { Check, ChevronDown } from "lucide-react";
import { cn } from "@/lib/utils";

export function LanguageSelector() {
  const { lang, setLang } = useLanguage();
  const current = LANGUAGE_OPTIONS.find((o) => o.code === lang) ?? LANGUAGE_OPTIONS[0];

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="h-8 px-2 gap-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground hover:text-foreground"
        >
          {current.label}
          <ChevronDown className="h-3.5 w-3.5 opacity-70" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-[10rem]">
        {LANGUAGE_OPTIONS.map((opt) => (
          <DropdownMenuItem
            key={opt.code}
            onClick={() => setLang(opt.code as Lang)}
            className={cn(
              "flex items-center justify-between gap-2 cursor-pointer",
              lang === opt.code && "text-primary"
            )}
          >
            <span>
              <span className="font-semibold">{opt.label}</span>
              <span className="text-muted-foreground"> – {opt.name}</span>
            </span>
            {lang === opt.code && <Check className="h-3.5 w-3.5" />}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
