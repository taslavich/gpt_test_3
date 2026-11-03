import { publisherInlineText } from "@/lib/publisherInlineZh";
import { useState, useEffect } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { PasswordInput } from "@/components/ui/password-input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  User,
  Bell,
  Shield,
  Save,
  Palette,
  Moon,
  Sun,
  Check,
  Wallet,
} from "lucide-react";
import { toast } from "sonner";
import { useLanguage } from "@/contexts/LanguageContext";
import { useProfile } from "@/contexts/ProfileContext";
import { api } from "@/api";
import { notifyError } from "@/lib/apiStatus";
import {
  useDashboardTheme,
  type DashboardTheme,
} from "@/contexts/DashboardThemeContext";
import { cn } from "@/lib/utils";
import { PAYOUT_METHODS } from "@/lib/publisherPreferences";
import { publisherApi, type PayoutMethod } from "@/api/publisher";
import { ApiError } from "@/api/http";
import {
  usePublisher,
  usePublisherResource,
} from "@/contexts/PublisherContext";
import { publisherTimezone } from "@/lib/publisherData";
import { PublisherError } from "@/components/publisher/PublisherApiState";
import { publisherUiText as tr } from "@/lib/publisherUiText";

export default function PublisherSettings() {
  const { t, lang } = useLanguage();
  const ru = lang === "ru";
  const { profile, loading, updateProfile } = useProfile();
  const { theme, setTheme } = useDashboardTheme();

  const [contactName, setContactName] = useState("");
  const [email, setEmail] = useState("");
  const [telegram, setTelegram] = useState("");
  const [timezone, setTimezone] = useState("UTC");
  const [codeAlerts, setCodeAlerts] = useState(false);
  const [payoutMethod, setPayoutMethod] = useState<PayoutMethod | "">("");
  const [wallet, setWallet] = useState("");
  const [saving, setSaving] = useState(false);
  const { refreshPublisher } = usePublisher();
  const preferences = usePublisherResource(["settings"], publisherApi.settings);
  const [preferenceError, setPreferenceError] = useState<unknown>(null);
  const [savingPreferences, setSavingPreferences] = useState(false);

  useEffect(() => {
    if (profile) {
      setContactName(profile.fullName || "");
      setEmail(profile.email || "");
      setTelegram(profile.telegram || "");
      try {
        setTimezone(publisherTimezone(profile.timezone));
      } catch (error) {
        setPreferenceError(error);
      }
    }
  }, [profile]);

  useEffect(() => {
    if (preferences.data) {
      setCodeAlerts(preferences.data.codeAlerts);
      setPayoutMethod(preferences.data.payoutMethod);
      setWallet(preferences.data.wallet);
    }
  }, [preferences.data]);

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

  const savePreferences = async () => {
    if (savingPreferences) return;
    if (!payoutMethod) {
      setPreferenceError(
        new ApiError(
          400,
          tr(
            lang,
            "Выберите сеть выплат",
            "Choose a payout network",
            "请选择提现网络",
          ),
        ),
      );
      return;
    }
    setSavingPreferences(true);
    setPreferenceError(null);
    try {
      await publisherApi.patchSettings({
        codeAlerts,
        payoutMethod,
        wallet: wallet.trim(),
      });
      await refreshPublisher();
      toast.success(t("settings.saved"));
    } catch (error) {
      setPreferenceError(error);
    } finally {
      setSavingPreferences(false);
    }
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
      toast.error(
        t("settings.passwordTooShort") ||
          "Password must be at least 6 characters",
      );
      return;
    }
    try {
      await api.changePassword({ new_password: newPassword });
      toast.success(t("settings.passwordUpdated"));
      setCurrentPassword("");
      setNewPassword("");
      setRepeatPassword("");
    } catch (e: unknown) {
      notifyError(t("settings.passwordError") || "Error updating password", e);
    }
  };

  if (loading)
    return (
      <div className="py-12 text-center text-muted-foreground">
        {publisherInlineText(lang, "Загрузка…", "Loading…")}
      </div>
    );

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold">{t("settings.title")}</h2>
        <p className="text-muted-foreground text-sm">
          {t("settings.subtitle")}
        </p>
      </div>

      {preferences.error && (
        <PublisherError error={preferences.error} retry={preferences.refetch} />
      )}
      {preferenceError && (
        <PublisherError
          error={preferenceError}
          retry={() => {
            setPreferenceError(null);
            return preferences.refetch();
          }}
        />
      )}
      <Tabs defaultValue="profile" className="min-w-0 space-y-6">
        <div className="max-w-full">
          <TabsList className="h-auto w-full flex-wrap justify-start gap-1 overflow-visible border border-border bg-card sm:w-auto sm:justify-start">
            <TabsTrigger value="profile" className="gap-2">
              <User className="h-4 w-4" /> {t("settings.profile")}
            </TabsTrigger>
            <TabsTrigger value="notifications" className="gap-2">
              <Bell className="h-4 w-4" /> {t("settings.notifications")}
            </TabsTrigger>
            <TabsTrigger value="security" className="gap-2">
              <Shield className="h-4 w-4" /> {t("settings.security")}
            </TabsTrigger>
            <TabsTrigger value="appearance" className="gap-2">
              <Palette className="h-4 w-4" /> {t("settings.appearance")}
            </TabsTrigger>
            <TabsTrigger value="payouts" className="gap-2">
              <Wallet className="h-4 w-4" />{" "}
              {publisherInlineText(lang, "Выплаты", "Payouts")}
            </TabsTrigger>
          </TabsList>
        </div>

        <TabsContent value="profile">
          <Card className="bg-card border-border">
            <CardHeader>
              <CardTitle className="text-lg">{t("settings.profile")}</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="grid sm:grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label htmlFor="publisher-profile-name">
                    {t("settings.name")}
                  </Label>
                  <Input
                    id="publisher-profile-name"
                    value={contactName}
                    onChange={(e) => setContactName(e.target.value)}
                    className="bg-background border-border"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="publisher-profile-email">
                    {t("settings.email")}
                  </Label>
                  <Input
                    id="publisher-profile-email"
                    value={email}
                    disabled
                    className="bg-muted border-border text-muted-foreground cursor-not-allowed"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="publisher-profile-telegram">
                    {t("settings.telegram")}
                  </Label>
                  <Input
                    id="publisher-profile-telegram"
                    value={telegram}
                    onChange={(e) => setTelegram(e.target.value)}
                    placeholder="@username"
                    className="bg-background border-border"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="publisher-profile-timezone">
                    {t("settings.timezone")}
                  </Label>
                  <Select value={timezone} onValueChange={setTimezone}>
                    <SelectTrigger
                      id="publisher-profile-timezone"
                      className="bg-background border-border"
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent className="bg-card border-border">
                      <SelectItem value="Etc/GMT+5">UTC-5 (EST)</SelectItem>
                      <SelectItem value="UTC">UTC+0</SelectItem>
                      <SelectItem value="Etc/GMT-1">UTC+1 (CET)</SelectItem>
                      <SelectItem value="Etc/GMT-2">UTC+2 (EET)</SelectItem>
                      <SelectItem value="Etc/GMT-3">UTC+3 (MSK)</SelectItem>
                      <SelectItem value="Asia/Kolkata">
                        UTC+5:30 (IST)
                      </SelectItem>
                      <SelectItem value="Etc/GMT-8">UTC+8 (CST)</SelectItem>
                      <SelectItem value="Etc/GMT-9">UTC+9 (JST)</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>
              <Button
                onClick={handleSaveProfile}
                disabled={saving}
                className="bg-primary hover:bg-primary/90"
              >
                <Save className="h-4 w-4 mr-2" />
                {t("settings.save")}
              </Button>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="notifications">
          <Card className="bg-card border-border">
            <CardHeader>
              <CardTitle className="text-lg">
                {t("settings.notifications")}
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-6">
              <div className="space-y-4">
                <h4 className="text-sm font-medium text-muted-foreground">
                  {t("settings.emailNotifications")}
                </h4>
                <div className="flex items-center justify-between gap-4">
                  <div>
                    <Label
                      htmlFor="publisher-code-alerts"
                      className="leading-5"
                    >
                      {publisherInlineText(
                        lang,
                        "Проблемы с рекламным кодом",
                        "Ad code issues",
                      )}
                    </Label>
                    <p className="mt-1 text-sm text-muted-foreground">
                      {tr(
                        lang,
                        "Уведомлять о прекращении запросов и восстановлении кода.",
                        "Notify me when code requests stop and resume.",
                        "广告代码请求停止或恢复时通知我。",
                      )}
                    </p>
                  </div>
                  <Switch
                    id="publisher-code-alerts"
                    checked={codeAlerts}
                    disabled={
                      preferences.isPending ||
                      !!preferences.error ||
                      savingPreferences
                    }
                    onCheckedChange={setCodeAlerts}
                  />
                </div>
              </div>
              <Button
                disabled={
                  preferences.isPending ||
                  !!preferences.error ||
                  savingPreferences
                }
                onClick={() => void savePreferences()}
                className="bg-primary hover:bg-primary/90"
              >
                <Save className="h-4 w-4 mr-2" />
                {t("settings.save")}
              </Button>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="security">
          <Card className="bg-card border-border">
            <CardHeader>
              <CardTitle className="text-lg">
                {t("settings.security")}
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-6">
              <div className="space-y-4">
                <div className="space-y-2">
                  <Label htmlFor="publisher-current-password">
                    {t("settings.currentPassword")}
                  </Label>
                  <PasswordInput
                    id="publisher-current-password"
                    placeholder="••••••••"
                    value={currentPassword}
                    onChange={(e) => setCurrentPassword(e.target.value)}
                    className="bg-background border-border"
                    containerClassName="max-w-sm"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="publisher-new-password">
                    {t("settings.newPassword")}
                  </Label>
                  <PasswordInput
                    id="publisher-new-password"
                    placeholder="••••••••"
                    value={newPassword}
                    onChange={(e) => setNewPassword(e.target.value)}
                    className="bg-background border-border"
                    containerClassName="max-w-sm"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="publisher-repeat-password">
                    {t("settings.repeatPassword")}
                  </Label>
                  <PasswordInput
                    id="publisher-repeat-password"
                    placeholder="••••••••"
                    value={repeatPassword}
                    onChange={(e) => setRepeatPassword(e.target.value)}
                    className="bg-background border-border"
                    containerClassName="max-w-sm"
                  />
                </div>
                <Button
                  onClick={handleChangePassword}
                  className="bg-primary hover:bg-primary/90"
                >
                  {t("settings.changePassword")}
                </Button>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="appearance">
          <Card className="bg-card border-border">
            <CardHeader>
              <CardTitle className="text-lg">
                {t("settings.appearance")}
              </CardTitle>
              <p className="text-sm text-muted-foreground">
                {t("settings.appearanceDescription")}
              </p>
            </CardHeader>
            <CardContent>
              <div className="grid max-w-2xl gap-4 sm:grid-cols-2">
                {[
                  {
                    value: "dark" as DashboardTheme,
                    label: t("settings.darkTheme"),
                    description: t("settings.darkThemeDescription"),
                    icon: Moon,
                  },
                  {
                    value: "light" as DashboardTheme,
                    label: t("settings.lightTheme"),
                    description: t("settings.lightThemeDescription"),
                    icon: Sun,
                  },
                ].map((option) => {
                  const selected = theme === option.value;
                  return (
                    <button
                      key={option.value}
                      type="button"
                      onClick={() => setTheme(option.value)}
                      aria-pressed={selected}
                      className={cn(
                        "relative min-w-0 rounded-xl border p-4 text-left transition-colors",
                        selected
                          ? "border-primary bg-primary/10"
                          : "border-border bg-background hover:border-primary/50",
                      )}
                    >
                      <div className="flex items-start gap-3">
                        <span
                          className={cn(
                            "rounded-lg p-2",
                            selected
                              ? "bg-primary text-primary-foreground"
                              : "bg-muted text-muted-foreground",
                          )}
                        >
                          <option.icon className="h-5 w-5" />
                        </span>
                        <div className="min-w-0 flex-1">
                          <p className="font-medium">{option.label}</p>
                          <p className="mt-1 text-sm text-muted-foreground">
                            {option.description}
                          </p>
                        </div>
                        {selected && (
                          <Check className="h-5 w-5 shrink-0 text-primary" />
                        )}
                      </div>
                    </button>
                  );
                })}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="payouts">
          <Card className="bg-card border-border">
            <CardHeader>
              <CardTitle className="text-lg">
                {publisherInlineText(
                  lang,
                  "Реквизиты выплат",
                  "Payout details",
                )}
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-6">
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="publisher-payout-method">
                    {tr(
                      lang,
                      "Валюта и сеть",
                      "Currency and network",
                      "币种和网络",
                    )}
                  </Label>
                  <Select
                    disabled={
                      preferences.isPending ||
                      !!preferences.error ||
                      savingPreferences
                    }
                    value={payoutMethod}
                    onValueChange={(value) =>
                      setPayoutMethod(value as PayoutMethod)
                    }
                  >
                    <SelectTrigger id="publisher-payout-method">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {PAYOUT_METHODS.map((method) => (
                        <SelectItem key={method.value} value={method.value}>
                          {method.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="publisher-wallet">
                    {tr(lang, "Адрес кошелька", "Wallet address", "钱包地址")}
                  </Label>
                  <Input
                    disabled={
                      preferences.isPending ||
                      !!preferences.error ||
                      savingPreferences
                    }
                    id="publisher-wallet"
                    value={wallet}
                    onChange={(event) => setWallet(event.target.value)}
                    placeholder={tr(
                      lang,
                      "Введите адрес кошелька",
                      "Enter a wallet address",
                      "输入钱包地址",
                    )}
                  />
                </div>
              </div>
              <p className="text-xs text-muted-foreground">
                {tr(
                  lang,
                  "Сохранённые реквизиты можно выбрать при создании заявки. Реквизиты уже созданных заявок не изменятся.",
                  "Select saved details when creating a request. Existing requests keep their original details.",
                  "创建申请时可选择已保存的信息。现有申请的收款信息不会更改。",
                )}
              </p>
              <Button
                disabled={
                  preferences.isPending ||
                  !!preferences.error ||
                  savingPreferences
                }
                onClick={() => void savePreferences()}
                className="bg-primary hover:bg-primary/90"
              >
                <Save className="h-4 w-4 mr-2" />
                {t("settings.save")}
              </Button>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
