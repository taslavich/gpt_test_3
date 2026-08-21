import { useState } from "react";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { PasswordInput } from "@/components/ui/password-input";
import { Label } from "@/components/ui/label";
import { Checkbox } from "@/components/ui/checkbox";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Link, useNavigate } from "react-router-dom";
import { useLanguage } from "@/contexts/LanguageContext";
import { useAuth } from "@/contexts/AuthContext";
import { toast } from "sonner";
import { MailCheck } from "lucide-react";
import { requestLoginIntro } from "@/lib/loginIntro";
import twinbidLogo from "@/assets/twinbid-logo.svg";

const authInputClass = "h-12 rounded-xl border-white/10 bg-white/[0.035] px-4 text-[15px] text-foreground placeholder:text-muted-foreground/65 focus-visible:border-primary/55 focus-visible:ring-1 focus-visible:ring-primary/35 focus-visible:ring-offset-0";
const authLabelClass = "text-[12px] font-semibold uppercase tracking-[0.09em] text-foreground/70";

interface AuthDialogProps {
  trigger?: React.ReactNode;
  defaultTab?: "login" | "register";
}

export function AuthDialog({ trigger, defaultTab = "login" }: AuthDialogProps) {
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [loginEmail, setLoginEmail] = useState("");
  const [loginPassword, setLoginPassword] = useState("");
  const [emailConfirmationRequired, setEmailConfirmationRequired] = useState(false);
  const [regName, setRegName] = useState("");
  const [regTelegram, setRegTelegram] = useState("");
  const [regEmail, setRegEmail] = useState("");
  const [regPassword, setRegPassword] = useState("");
  const [regConfirm, setRegConfirm] = useState("");
  const [regConsent, setRegConsent] = useState(false);
  const navigate = useNavigate();
  const { t } = useLanguage();
  const { signIn, signUp } = useAuth();

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault();
    setEmailConfirmationRequired(false);
    setLoading(true);
    const { error } = await signIn(loginEmail, loginPassword);
    setLoading(false);
    if (error) {
      if (error === t("auth.error.confirmEmail")) {
        setEmailConfirmationRequired(true);
      }
      toast.error(error);
      return;
    }
    requestLoginIntro();
    setOpen(false);
    navigate("/dashboard");
  };

  const handleRegister = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!regConsent) {
      toast.error(t("auth.consent.required"));
      return;
    }
    if (!regTelegram.trim()) {
      toast.error(t("auth.telegramRequired"));
      return;
    }
    if (regPassword !== regConfirm) {
      toast.error(t("auth.passwordMismatch") || "Passwords do not match");
      return;
    }
    if (regPassword.length < 6) {
      toast.error(t("auth.passwordTooShort") || "Password must be at least 6 characters");
      return;
    }
    setLoading(true);
    const { error } = await signUp(regEmail, regPassword, regName, regTelegram.trim());
    setLoading(false);
    if (error) {
      toast.error(error);
      return;
    }
    toast.success(t("auth.checkEmail") || "Check your email to confirm your account");
    setOpen(false);
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        {trigger || <Button>{t("nav.login")}</Button>}
      </DialogTrigger>
      <DialogContent className="landing-auth-dialog gap-5 border-white/10 bg-[hsl(200_18%_6%)] p-5 shadow-[0_32px_100px_rgba(0,0,0,0.62)] sm:max-w-[480px] sm:rounded-[24px] sm:p-7">
        <DialogHeader className="items-center border-b border-white/[0.08] pb-5 text-center">
          <DialogTitle className="flex items-center justify-center">
            <img src={twinbidLogo} alt="TwinBid" className="h-11 w-auto" />
          </DialogTitle>
        </DialogHeader>
        <Tabs defaultValue={defaultTab} className="w-full">
          <TabsList className="grid h-12 w-full grid-cols-2 rounded-xl border border-white/[0.08] bg-black/20 p-1">
            <TabsTrigger value="login" className="h-10 rounded-lg text-[13px] font-semibold data-[state=active]:bg-primary data-[state=active]:text-primary-foreground data-[state=active]:shadow-none">{t("auth.login")}</TabsTrigger>
            <TabsTrigger value="register" className="h-10 rounded-lg text-[13px] font-semibold data-[state=active]:bg-primary data-[state=active]:text-primary-foreground data-[state=active]:shadow-none">{t("auth.register")}</TabsTrigger>
          </TabsList>
          <TabsContent value="login" className="mt-5 space-y-4">
            <form onSubmit={handleLogin} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="email-login" className={authLabelClass}>{t("auth.email")}</Label>
                <Input id="email-login" type="email" placeholder="your@email.com" className={authInputClass}
                  value={loginEmail} onChange={(e) => { setLoginEmail(e.target.value); setEmailConfirmationRequired(false); }} required />
              </div>
              <div className="space-y-2">
                <Label htmlFor="password-login" className={authLabelClass}>{t("auth.password")}</Label>
                <PasswordInput id="password-login" placeholder="••••••••" className={authInputClass}
                  value={loginPassword} onChange={(e) => { setLoginPassword(e.target.value); setEmailConfirmationRequired(false); }} required />
                <p className="text-xs text-muted-foreground">{t("auth.passwordResetSupport")}</p>
              </div>
              {emailConfirmationRequired && (
                <div role="alert" className="flex gap-3 rounded-xl border border-primary/25 bg-primary/10 p-3.5 text-left">
                  <MailCheck className="mt-0.5 h-5 w-5 shrink-0 text-primary" />
                  <div>
                    <p className="text-sm font-semibold text-foreground">{t("auth.error.confirmEmailTitle")}</p>
                    <p className="mt-1 text-xs leading-5 text-muted-foreground">{t("auth.error.confirmEmail")}</p>
                  </div>
                </div>
              )}
              <Button type="submit" className="h-12 w-full rounded-xl bg-primary text-[14px] font-semibold text-primary-foreground hover:bg-primary/90" disabled={loading}>
                {loading ? "..." : t("auth.loginBtn")}
              </Button>
            </form>
          </TabsContent>
          <TabsContent value="register" className="mt-5 space-y-4">
            <form onSubmit={handleRegister} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="name" className={authLabelClass}>{t("auth.name")}</Label>
                <Input id="name" placeholder="John Doe" className={authInputClass}
                  value={regName} onChange={(e) => setRegName(e.target.value)} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="email-register" className={authLabelClass}>{t("auth.email")}</Label>
                <Input id="email-register" type="email" placeholder="your@email.com" className={authInputClass}
                  value={regEmail} onChange={(e) => setRegEmail(e.target.value)} required />
              </div>
              <div className="space-y-2">
                <Label htmlFor="password-register" className={authLabelClass}>{t("auth.password")}</Label>
                <PasswordInput id="password-register" placeholder="••••••••" className={authInputClass}
                  value={regPassword} onChange={(e) => setRegPassword(e.target.value)} required />
              </div>
              <div className="space-y-2">
                <Label htmlFor="password-confirm" className={authLabelClass}>{t("auth.confirmPassword")}</Label>
                <PasswordInput id="password-confirm" placeholder="••••••••" className={authInputClass}
                  value={regConfirm} onChange={(e) => setRegConfirm(e.target.value)} required />
              </div>
              <div className="space-y-2">
                <Label htmlFor="telegram-register" className={authLabelClass}>{t("auth.telegram")}</Label>
                <Input id="telegram-register" placeholder="@username" className={authInputClass}
                  value={regTelegram} onChange={(e) => setRegTelegram(e.target.value)} required />
              </div>
              <div className="flex items-start gap-2 pt-1">
                <Checkbox
                  id="reg-consent"
                  checked={regConsent}
                  onCheckedChange={(v) => setRegConsent(v === true)}
                  className="mt-0.5"
                />
                <Label htmlFor="reg-consent" className="text-xs leading-snug font-normal text-muted-foreground cursor-pointer">
                  {t("auth.consent.intro")}
                  <Link to="/legal#terms" target="_blank" className="text-primary hover:underline" onClick={(e) => e.stopPropagation()}>
                    {t("auth.consent.terms")}
                  </Link>
                  {" "}{t("auth.consent.and")}{" "}
                  <Link to="/legal#privacy" target="_blank" className="text-primary hover:underline" onClick={(e) => e.stopPropagation()}>
                    {t("auth.consent.privacy")}
                  </Link>
                  {t("auth.consent.outro")}
                </Label>
              </div>
              <Button type="submit" className="h-12 w-full rounded-xl bg-primary text-[14px] font-semibold text-primary-foreground hover:bg-primary/90" disabled={loading || !regConsent}>
                {loading ? "..." : t("auth.registerBtn")}
              </Button>
            </form>
          </TabsContent>
        </Tabs>
      </DialogContent>
    </Dialog>
  );
}
