import { useState, type FormEvent } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";
import { ArrowRight, Check, Sparkles } from "lucide-react";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { PasswordInput } from "@/components/ui/password-input";
import { useAuth } from "@/contexts/AuthContext";
import type { Lang } from "@/contexts/LanguageContext";

const AUTH_COPY = {
  en: {
    consentError: "Please accept the Terms and Privacy Policy.", telegramError: "Enter your Telegram username.", mismatch: "Passwords do not match.", short: "Password must be at least 6 characters.", success: "Your account is almost ready. Confirm your email to continue.", title: "Launch your first campaign", subtitle: "Create your TwinBid account and get access to worldwide traffic.", fast: "Fast setup", tools: "Built-in tools", name: "Name", telegram: "Telegram", email: "Email", password: "Password", confirm: "Confirm", passwordPlaceholder: "6+ characters", repeat: "Repeat password", agree: "I agree to the", terms: "Terms of Use", and: "and", privacy: "Privacy Policy", creating: "CREATING ACCOUNT...", create: "CREATE FREE ACCOUNT",
  },
  ru: {
    consentError: "Примите Условия использования и Политику конфиденциальности.", telegramError: "Укажите имя пользователя в Telegram.", mismatch: "Пароли не совпадают.", short: "Пароль должен содержать не менее 6 символов.", success: "Аккаунт почти готов. Подтвердите почту, чтобы продолжить.", title: "Запустите первую кампанию", subtitle: "Создайте аккаунт TwinBid и получите доступ к мировому трафику.", fast: "Быстрая настройка", tools: "Встроенные инструменты", name: "Имя", telegram: "Telegram", email: "Почта", password: "Пароль", confirm: "Повторите пароль", passwordPlaceholder: "Не менее 6 символов", repeat: "Повторите пароль", agree: "Я принимаю", terms: "Условия использования", and: "и", privacy: "Политику конфиденциальности", creating: "СОЗДАЁМ АККАУНТ...", create: "СОЗДАТЬ БЕСПЛАТНЫЙ АККАУНТ",
  },
  es: {
    consentError: "Acepta los Términos y la Política de Privacidad.", telegramError: "Introduce tu usuario de Telegram.", mismatch: "Las contraseñas no coinciden.", short: "La contraseña debe tener al menos 6 caracteres.", success: "Tu cuenta está casi lista. Confirma tu correo para continuar.", title: "Lanza tu primera campaña", subtitle: "Crea tu cuenta TwinBid y accede al tráfico mundial.", fast: "Configuración rápida", tools: "Herramientas integradas", name: "Nombre", telegram: "Telegram", email: "Correo", password: "Contraseña", confirm: "Confirmar", passwordPlaceholder: "6+ caracteres", repeat: "Repite la contraseña", agree: "Acepto los", terms: "Términos de Uso", and: "y la", privacy: "Política de Privacidad", creating: "CREANDO CUENTA...", create: "CREAR CUENTA GRATIS",
  },
} as const;

interface MoneyRegisterDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  lang: Lang;
}

export function MoneyRegisterDialog({ open, onOpenChange, lang }: MoneyRegisterDialogProps) {
  const { signUp } = useAuth();
  const copy = AUTH_COPY[lang];
  const [loading, setLoading] = useState(false);
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [telegram, setTelegram] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [consent, setConsent] = useState(false);

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault();
    if (!consent) {
      toast.error(copy.consentError);
      return;
    }
    if (!telegram.trim()) {
      toast.error(copy.telegramError);
      return;
    }
    if (password !== confirmPassword) {
      toast.error(copy.mismatch);
      return;
    }
    if (password.length < 6) {
      toast.error(copy.short);
      return;
    }

    setLoading(true);
    const { error } = await signUp(email, password, name || undefined, telegram.trim());
    setLoading(false);

    if (error) {
      toast.error(error);
      return;
    }

    toast.success(copy.success);
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="money-register-dialog max-h-[92svh] overflow-y-auto border-violet-400/20 bg-[#0b0d12] p-0 text-white sm:max-w-[460px]">
        <div className="border-b border-white/10 bg-[radial-gradient(circle_at_top_right,rgba(139,92,246,0.22),transparent_45%)] px-6 pb-5 pt-6">
          <DialogHeader>
            <div className="mb-3 flex h-10 w-10 items-center justify-center rounded-xl bg-violet-500 text-sm font-black text-white shadow-[0_0_30px_rgba(139,92,246,0.35)]">
              T
            </div>
            <DialogTitle className="text-left text-2xl font-semibold tracking-[-0.03em] text-white">
              {copy.title}
            </DialogTitle>
            <p className="text-left text-sm leading-6 text-white/55">
              {copy.subtitle}
            </p>
          </DialogHeader>
          <div className="mt-4 flex flex-wrap gap-2 text-xs text-white/65">
            <span className="inline-flex items-center gap-1.5 rounded-full border border-white/10 bg-white/[0.04] px-3 py-1.5"><Check className="h-3.5 w-3.5 text-emerald-400" /> {copy.fast}</span>
            <span className="inline-flex items-center gap-1.5 rounded-full border border-white/10 bg-white/[0.04] px-3 py-1.5"><Sparkles className="h-3.5 w-3.5 text-violet-400" /> {copy.tools}</span>
          </div>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4 px-6 pb-6 pt-5">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="money-name" className="text-white/75">{copy.name}</Label>
              <Input
                id="money-name"
                value={name}
                onChange={(event) => setName(event.target.value)}
                placeholder="John"
                className="border-white/10 bg-white/[0.04] text-white placeholder:text-white/25"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="money-telegram" className="text-white/75">{copy.telegram}</Label>
              <Input
                id="money-telegram"
                value={telegram}
                onChange={(event) => setTelegram(event.target.value)}
                placeholder="@username"
                className="border-white/10 bg-white/[0.04] text-white placeholder:text-white/25"
                required
              />
            </div>
          </div>
          <div className="space-y-2">
            <Label htmlFor="money-email" className="text-white/75">{copy.email}</Label>
            <Input
              id="money-email"
              type="email"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              placeholder="you@email.com"
              className="border-white/10 bg-white/[0.04] text-white placeholder:text-white/25"
              required
            />
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="money-password" className="text-white/75">{copy.password}</Label>
              <PasswordInput
                id="money-password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                placeholder={copy.passwordPlaceholder}
                className="border-white/10 bg-white/[0.04] text-white placeholder:text-white/25"
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="money-password-confirm" className="text-white/75">{copy.confirm}</Label>
              <PasswordInput
                id="money-password-confirm"
                value={confirmPassword}
                onChange={(event) => setConfirmPassword(event.target.value)}
                placeholder={copy.repeat}
                className="border-white/10 bg-white/[0.04] text-white placeholder:text-white/25"
                required
              />
            </div>
          </div>
          <div className="flex items-start gap-2.5 pt-1">
            <Checkbox
              id="money-consent"
              checked={consent}
              onCheckedChange={(value) => setConsent(value === true)}
              className="mt-0.5 border-white/25 data-[state=checked]:border-violet-500 data-[state=checked]:bg-violet-500"
            />
            <Label htmlFor="money-consent" className="cursor-pointer text-xs font-normal leading-5 text-white/45">
              {copy.agree}{" "}
              <Link to="/legal#terms" target="_blank" className="text-violet-300 hover:text-violet-200">{copy.terms}</Link>
              {" "}{copy.and}{" "}
              <Link to="/legal#privacy" target="_blank" className="text-violet-300 hover:text-violet-200">{copy.privacy}</Link>.
            </Label>
          </div>
          <Button
            type="submit"
            disabled={loading || !consent}
            className="h-12 w-full rounded-xl bg-violet-500 text-sm font-bold text-white shadow-[0_15px_45px_rgba(139,92,246,0.25)] hover:bg-violet-400"
          >
            {loading ? copy.creating : copy.create}
            {!loading && <ArrowRight className="ml-2 h-4 w-4" />}
          </Button>
        </form>
      </DialogContent>
    </Dialog>
  );
}
