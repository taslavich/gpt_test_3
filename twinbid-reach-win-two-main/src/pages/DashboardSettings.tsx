import { useState, useEffect } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { PasswordInput } from "@/components/ui/password-input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { User, Bell, Shield, Save, Palette, Moon, Sun, Check } from "lucide-react";
import { toast } from "sonner";
import { useLanguage } from "@/contexts/LanguageContext";
import { useProfile } from "@/contexts/ProfileContext";
import { api } from "@/api";
import { notifyError } from "@/lib/apiStatus";
import { useDashboardTheme, type DashboardTheme } from "@/contexts/DashboardThemeContext";
import { cn } from "@/lib/utils";

export default function DashboardSettings() {
  const { t } = useLanguage();
  const { profile, loading, updateProfile } = useProfile();
  const { theme, setTheme } = useDashboardTheme();

  const [contactName, setContactName] = useState("");
  const [email, setEmail] = useState("");
  const [telegram, setTelegram] = useState("");
  const [timezone, setTimezone] = useState("utc_3");
  const [notifyCampaign, setNotifyCampaign] = useState(true);
  const [notifyBalance, setNotifyBalance] = useState(true);
  const [balanceThreshold, setBalanceThreshold] = useState("100");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (profile) {
      setContactName(profile.fullName || "");
      setEmail(profile.email || "");
      setTelegram(profile.telegram || "");
      setTimezone(profile.timezone || "utc_3");
      setNotifyCampaign(profile.notifyCampaignStatus);
      setNotifyBalance(profile.notifyLowBalance);
      setBalanceThreshold(String(profile.balanceThreshold));
    }
  }, [profile]);

  const handleSaveProfile = async () => {
    setSaving(true);
    try {
      await updateProfile({
        fullName: contactName,
        telegram,
        timezone,
      });
      toast.success(t("settings.saved"));
    } catch (err) {
      notifyError(t("settings.saveError") || "Error saving profile", err);
    }
    setSaving(false);
  };

  const handleSaveNotifications = async () => {
    setSaving(true);
    try {
      await updateProfile({
        notifyCampaignStatus: notifyCampaign,
        notifyLowBalance: notifyBalance,
        balanceThreshold: parseInt(balanceThreshold) || 100,
      });
      toast.success(t("settings.saved"));
    } catch (err) {
      notifyError(t("settings.saveError") || "Error saving notifications", err);
    }
    setSaving(false);
  };

  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [repeatPassword, setRepeatPassword] = useState("");

  const handleChangePassword = async () => {
    if (newPassword !== repeatPassword) {
      toast.error(t("settings.passwordMismatch") || "Passwords don't match");
      return;
    }
    if (newPassword.length < 6) {
      toast.error(t("settings.passwordTooShort") || "Password must be at least 6 characters");
      return;
    }
    try {
      await api.changePassword({ new_password: newPassword });
      toast.success(t("settings.passwordUpdated"));
      setCurrentPassword("");
      setNewPassword("");
      setRepeatPassword("");
    } catch (e: any) {
      notifyError(t("settings.passwordError") || "Error updating password", e);
    }
  };

  if (loading) return <div className="py-12 text-center text-muted-foreground">Loading...</div>;

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold">{t("settings.title")}</h2>
        <p className="text-muted-foreground text-sm">{t("settings.subtitle")}</p>
      </div>

      <Tabs defaultValue="profile" className="min-w-0 space-y-6">
        <TabsList className="w-full justify-start overflow-x-auto border border-border bg-card sm:w-auto">
          <TabsTrigger value="profile" className="gap-2"><User className="h-4 w-4" /> {t("settings.profile")}</TabsTrigger>
          <TabsTrigger value="notifications" className="gap-2"><Bell className="h-4 w-4" /> {t("settings.notifications")}</TabsTrigger>
          <TabsTrigger value="security" className="gap-2"><Shield className="h-4 w-4" /> {t("settings.security")}</TabsTrigger>
          <TabsTrigger value="appearance" className="gap-2"><Palette className="h-4 w-4" /> {t("settings.appearance")}</TabsTrigger>
        </TabsList>

        <TabsContent value="profile">
          <Card className="bg-card border-border">
            <CardHeader><CardTitle className="text-lg">{t("settings.profile")}</CardTitle></CardHeader>
            <CardContent className="space-y-4">
              <div className="grid sm:grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label>{t("settings.name")}</Label>
                  <Input value={contactName} onChange={(e) => setContactName(e.target.value)} className="bg-background border-border" />
                </div>
                <div className="space-y-2">
                  <Label>{t("settings.email")}</Label>
                  <Input value={email} disabled className="bg-muted border-border text-muted-foreground cursor-not-allowed" />
                </div>
                <div className="space-y-2">
                  <Label>{t("settings.telegram")}</Label>
                  <Input value={telegram} onChange={(e) => setTelegram(e.target.value)} placeholder="@username" className="bg-background border-border" />
                </div>
                <div className="space-y-2">
                  <Label>{t("settings.timezone")}</Label>
                  <Select value={timezone} onValueChange={setTimezone}>
                    <SelectTrigger className="bg-background border-border"><SelectValue /></SelectTrigger>
                    <SelectContent className="bg-card border-border">
                      <SelectItem value="utc_m5">UTC-5 (EST)</SelectItem>
                      <SelectItem value="utc_0">UTC+0 (GMT)</SelectItem>
                      <SelectItem value="utc_1">UTC+1 (CET)</SelectItem>
                      <SelectItem value="utc_2">UTC+2 (EET)</SelectItem>
                      <SelectItem value="utc_3">UTC+3 (MSK)</SelectItem>
                      <SelectItem value="utc_5_30">UTC+5:30 (IST)</SelectItem>
                      <SelectItem value="utc_8">UTC+8 (CST)</SelectItem>
                      <SelectItem value="utc_9">UTC+9 (JST)</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>
              <Button onClick={handleSaveProfile} disabled={saving} className="bg-primary hover:bg-primary/90">
                <Save className="h-4 w-4 mr-2" />{t("settings.save")}
              </Button>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="notifications">
          <Card className="bg-card border-border">
            <CardHeader><CardTitle className="text-lg">{t("settings.notifications")}</CardTitle></CardHeader>
            <CardContent className="space-y-6">
              <div className="space-y-4">
                <h4 className="text-sm font-medium text-muted-foreground">{t("settings.emailNotifications")}</h4>
                <div className="flex items-center justify-between gap-4">
                  <Label className="leading-5">{t("settings.campaignStatus")}</Label>
                  <Switch checked={notifyCampaign} onCheckedChange={setNotifyCampaign} />
                </div>
                <div className="flex items-center justify-between gap-4">
                  <Label className="leading-5">{t("settings.lowBalance")}</Label>
                  <Switch checked={notifyBalance} onCheckedChange={setNotifyBalance} />
                </div>
              </div>
              <Separator />
              <div className="space-y-2 max-w-xs">
                <Label>{t("settings.balanceThreshold")}</Label>
                <div className="relative">
                  <Input value={balanceThreshold} onChange={(e) => setBalanceThreshold(e.target.value)} className="bg-background border-border pr-8" />
                  <span className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground">$</span>
                </div>
              </div>
              <Button onClick={handleSaveNotifications} disabled={saving} className="bg-primary hover:bg-primary/90">
                <Save className="h-4 w-4 mr-2" />{t("settings.save")}
              </Button>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="security">
          <Card className="bg-card border-border">
            <CardHeader><CardTitle className="text-lg">{t("settings.security")}</CardTitle></CardHeader>
            <CardContent className="space-y-6">
              <div className="space-y-4">
                <div className="space-y-2">
                  <Label>{t("settings.currentPassword")}</Label>
                  <PasswordInput placeholder="••••••••" value={currentPassword} onChange={(e) => setCurrentPassword(e.target.value)} className="bg-background border-border" containerClassName="max-w-sm" />
                </div>
                <div className="space-y-2">
                  <Label>{t("settings.newPassword")}</Label>
                  <PasswordInput placeholder="••••••••" value={newPassword} onChange={(e) => setNewPassword(e.target.value)} className="bg-background border-border" containerClassName="max-w-sm" />
                </div>
                <div className="space-y-2">
                  <Label>{t("settings.repeatPassword")}</Label>
                  <PasswordInput placeholder="••••••••" value={repeatPassword} onChange={(e) => setRepeatPassword(e.target.value)} className="bg-background border-border" containerClassName="max-w-sm" />
                </div>
                <Button onClick={handleChangePassword} className="bg-primary hover:bg-primary/90">{t("settings.changePassword")}</Button>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="appearance">
          <Card className="bg-card border-border">
            <CardHeader>
              <CardTitle className="text-lg">{t("settings.appearance")}</CardTitle>
              <p className="text-sm text-muted-foreground">{t("settings.appearanceDescription")}</p>
            </CardHeader>
            <CardContent>
              <div className="grid max-w-2xl gap-4 sm:grid-cols-2">
                {([
                  { value: "dark" as DashboardTheme, label: t("settings.darkTheme"), description: t("settings.darkThemeDescription"), icon: Moon },
                  { value: "light" as DashboardTheme, label: t("settings.lightTheme"), description: t("settings.lightThemeDescription"), icon: Sun },
                ]).map((option) => {
                  const selected = theme === option.value;
                  return (
                    <button
                      key={option.value}
                      type="button"
                      onClick={() => setTheme(option.value)}
                      aria-pressed={selected}
                      className={cn(
                        "relative min-w-0 rounded-xl border p-4 text-left transition-colors",
                        selected ? "border-primary bg-primary/10" : "border-border bg-background hover:border-primary/50",
                      )}
                    >
                      <div className="flex items-start gap-3">
                        <span className={cn("rounded-lg p-2", selected ? "bg-primary text-primary-foreground" : "bg-muted text-muted-foreground")}>
                          <option.icon className="h-5 w-5" />
                        </span>
                        <div className="min-w-0 flex-1">
                          <p className="font-medium">{option.label}</p>
                          <p className="mt-1 text-sm text-muted-foreground">{option.description}</p>
                        </div>
                        {selected && <Check className="h-5 w-5 shrink-0 text-primary" />}
                      </div>
                    </button>
                  );
                })}
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
