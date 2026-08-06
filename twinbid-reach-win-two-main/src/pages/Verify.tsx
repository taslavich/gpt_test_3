import { useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { CheckCircle2, AlertTriangle, XCircle, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useLanguage } from "@/contexts/LanguageContext";
import { api, ApiError } from "@/api";

type VerifyState = "loading" | "success" | "already" | "invalid";

export default function Verify() {
  const [params] = useSearchParams();
  const token = params.get("token") || "";
  const { t } = useLanguage();
  const [state, setState] = useState<VerifyState>("loading");

  useEffect(() => {
    let cancelled = false;
    if (!token) {
      setState("invalid");
      return;
    }
    (async () => {
      try {
        await api.verifyEmail({ token });
        if (!cancelled) setState("success");
      } catch (e) {
        if (cancelled) return;
        if (e instanceof ApiError && e.status === 409) setState("already");
        else setState("invalid");
      }
    })();
    return () => { cancelled = true; };
  }, [token]);

  const config = {
    loading: {
      icon: <Loader2 className="h-16 w-16 text-primary animate-spin" />,
      title: t("verify.loading"),
      desc: "",
      accent: "from-primary/20 to-accent/20",
    },
    success: {
      icon: <CheckCircle2 className="h-16 w-16 text-primary" />,
      title: t("verify.success.title"),
      desc: t("verify.success.desc"),
      accent: "from-primary/30 to-accent/20",
    },
    already: {
      icon: <AlertTriangle className="h-16 w-16 text-accent" />,
      title: t("verify.already.title"),
      desc: t("verify.already.desc"),
      accent: "from-accent/30 to-primary/20",
    },
    invalid: {
      icon: <XCircle className="h-16 w-16 text-destructive" />,
      title: t("verify.invalid.title"),
      desc: t("verify.invalid.desc"),
      accent: "from-destructive/20 to-accent/10",
    },
  }[state];

  return (
    <div className="min-h-screen flex items-center justify-center bg-background px-4 relative overflow-hidden">
      {/* Decorative background */}
      <div className="absolute inset-0 -z-10">
        <div className="absolute top-1/4 left-1/4 w-[500px] h-[500px] rounded-full bg-primary/10 blur-[120px]" />
        <div className="absolute bottom-1/4 right-1/4 w-[500px] h-[500px] rounded-full bg-accent/10 blur-[120px]" />
      </div>

      <div className="w-full max-w-md">
        <div className={`relative rounded-2xl border border-border bg-card/80 backdrop-blur-xl p-8 shadow-2xl bg-gradient-to-br ${config.accent}`}>
          <div className="flex flex-col items-center text-center space-y-6">
            <div className="rounded-full bg-background/60 p-5 ring-1 ring-border">
              {config.icon}
            </div>
            <div className="space-y-2">
              <h1 className="text-2xl font-bold bg-gradient-to-r from-primary to-accent bg-clip-text text-transparent">
                {config.title}
              </h1>
              {config.desc && (
                <p className="text-muted-foreground text-sm leading-relaxed">{config.desc}</p>
              )}
            </div>

            {state !== "loading" && (
              <div className="flex flex-col sm:flex-row gap-3 w-full pt-2">
                <Button asChild variant="outline" className="flex-1">
                  <Link to="/">{t("verify.goHome")}</Link>
                </Button>
                {state === "success" || state === "already" ? (
                  <Button asChild className="flex-1 bg-primary hover:bg-primary/90">
                    <Link to="/">{t("verify.goLogin")}</Link>
                  </Button>
                ) : null}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
