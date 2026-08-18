import { cn } from "@/lib/utils";

export function PassimPayBrand({ className }: { className?: string }) {
  return (
    <span className={cn("inline-flex items-center gap-2", className)} aria-label="PassimPay">
      <svg viewBox="0 0 42 30" aria-hidden="true" className="h-7 w-10 shrink-0">
        <path fill="#ff6508" d="M8 4h11l-6 7H2L8 4Zm11 0h10.5c6.5 0 10.5 3.5 10.5 8.8 0 6.5-5 12.2-13.7 12.2h-5.5l-4.3 5H6l15.7-18.4h8.1c1.8 0 2.8-.8 2.8-2 0-1.1-.9-1.6-2.6-1.6h-7.6L9.2 23H0L19 4Z" />
      </svg>
      <span className="whitespace-nowrap text-xl font-extrabold italic tracking-tight text-[#1855c5] dark:text-[#70a0ff]">
        passimpay<span className="text-base not-italic">.io</span>
      </span>
    </span>
  );
}
