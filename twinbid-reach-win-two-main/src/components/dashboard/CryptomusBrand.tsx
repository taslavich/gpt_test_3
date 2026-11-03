export function CryptomusBrand() {
  return (
    <span className="flex min-w-0 items-center gap-2.5" aria-label="Cryptomus">
      <span className="flex h-10 w-10 shrink-0 items-center justify-center text-foreground">
        <svg viewBox="0 0 34 34" aria-hidden="true" className="h-9 w-9">
          <path d="M17 2.5 31 10.3 17 18 3 10.3 17 2.5Z" fill="none" stroke="currentColor" strokeWidth="2.8" strokeLinejoin="round" />
          <path d="M3 10.3 17 18v13.5L3 23.7V10.3Z" fill="none" stroke="currentColor" strokeWidth="2.8" strokeLinejoin="round" />
          <path d="M17 18 31 10.3v13.4l-14 7.8V18Z" fill="currentColor" stroke="currentColor" strokeWidth="2.8" strokeLinejoin="round" />
        </svg>
      </span>
      <span className="truncate text-lg font-bold tracking-[-0.04em] text-foreground">cryptomus</span>
    </span>
  );
}
