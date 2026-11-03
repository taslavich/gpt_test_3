import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { PostbackSection } from "@/components/dashboard/PostbackSection";
import { useLanguage } from "@/contexts/LanguageContext";

export default function DashboardPostback() {
  const { t } = useLanguage();

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <div>
        <h2 className="text-2xl font-bold">{t("postback.pageTitle")}</h2>
        <p className="text-sm text-muted-foreground">{t("postback.pageSubtitle")}</p>
      </div>

      <Card className="border-border bg-card">
        <CardHeader>
          <CardTitle className="text-lg">{t("postback.setupTitle")}</CardTitle>
        </CardHeader>
        <CardContent>
          <PostbackSection />
        </CardContent>
      </Card>
    </div>
  );
}
