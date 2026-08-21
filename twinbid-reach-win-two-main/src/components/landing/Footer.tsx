import { useState } from "react";
import { Mail, Send } from "lucide-react";
import { Link } from "react-router-dom";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Copy } from "lucide-react";
import { toast } from "sonner";
import { useLanguage } from "@/contexts/LanguageContext";
import twinbidLogo from "@/assets/twinbid-logo.svg";

export function Footer() {
  const { t, lang } = useLanguage();
  const [emailOpen, setEmailOpen] = useState(false);
  const email = "twinbid@twinbidex.com";

  const copyEmail = () => {
    navigator.clipboard.writeText(email);
    toast.success("Email скопирован");
  };

  return (
    <footer className="px-5 pb-5 pt-10 md:px-8">
      <div className="mx-auto max-w-[1280px] rounded-[28px] border border-white/[0.08] bg-white/[0.02] px-6 py-8 md:px-9">
        <div className="flex flex-col items-center gap-8 md:flex-row">
          <div className="flex-1 flex flex-col items-center md:items-start gap-2">
            <a href="#" className="flex items-center gap-2">
              <img src={twinbidLogo} alt="TwinBid" className="h-10" />
            </a>
            <p className="text-sm text-muted-foreground">© {new Date().getFullYear()} TwinBid. {lang === "ru" ? "Все права защищены." : "All rights reserved."}</p>
          </div>
          <div className="flex-none flex flex-wrap items-center justify-center gap-5 text-sm">
            <Link to="/legal#privacy" className="text-muted-foreground hover:text-foreground transition-colors">{t("footer.privacy")}</Link>
            <Link to="/legal#terms" className="text-muted-foreground hover:text-foreground transition-colors">{t("footer.terms")}</Link>
            <a href="#" className="text-muted-foreground hover:text-foreground transition-colors">{t("footer.docs")}</a>
          </div>
          <div className="flex-1 flex items-center justify-center md:justify-end gap-4">
            <button onClick={() => setEmailOpen(true)} className="flex h-10 w-10 items-center justify-center rounded-full border border-white/10 bg-white/[0.025] text-muted-foreground transition-colors hover:border-primary/40 hover:text-foreground" title="Email">
              <Mail className="w-5 h-5" />
            </button>
            <a href="https://t.me/GregTwinbid" target="_blank" rel="noopener noreferrer" className="flex h-10 w-10 items-center justify-center rounded-full border border-white/10 bg-white/[0.025] text-muted-foreground transition-colors hover:border-primary/40 hover:text-foreground" title="Telegram: @GregTwinbid">
              <Send className="w-5 h-5" />
            </a>
          </div>
        </div>
      </div>

      <Dialog open={emailOpen} onOpenChange={setEmailOpen}>
        <DialogContent className="sm:max-w-[400px] bg-card border-border">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2"><Mail className="h-5 w-5" /> Email</DialogTitle>
          </DialogHeader>
          <div className="flex items-center gap-2 mt-2">
            <span className="flex-1 font-mono text-sm bg-background border border-border rounded-lg px-3 py-2">{email}</span>
            <Button variant="outline" size="icon" onClick={copyEmail} className="border-border shrink-0">
              <Copy className="h-4 w-4" />
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </footer>
  );
}
