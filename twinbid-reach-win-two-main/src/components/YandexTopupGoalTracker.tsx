import { useEffect } from "react";
import { api } from "@/api";
import { useAuth } from "@/contexts/AuthContext";
import {
  discardTopupFromGoal,
  hasPendingTopupsForGoal,
  trackBalanceTopupSuccess,
} from "@/lib/yandexMetrikaTopup";

const TOPUP_STATUS_CHECK_INTERVAL_MS = 60_000;

/** Tracks credited top-ups even if approval happens outside the Balance page. */
export function YandexTopupGoalTracker() {
  const { user } = useAuth();

  useEffect(() => {
    if (!user) return;
    let cancelled = false;

    const checkPendingTopups = async () => {
      if (!hasPendingTopupsForGoal()) return;
      try {
        const response = await api.listTransactions();
        if (cancelled) return;
        const transactions = Array.isArray(response?.items) ? response.items : [];
        transactions.forEach(transaction => {
          if (transaction.status === "rejected" || transaction.status === "cancelled") {
            discardTopupFromGoal(transaction.id);
            return;
          }
          trackBalanceTopupSuccess(transaction);
        });
      } catch (error) {
        console.error("Yandex top-up goal status check failed", error);
      }
    };

    void checkPendingTopups();
    const interval = window.setInterval(checkPendingTopups, TOPUP_STATUS_CHECK_INTERVAL_MS);
    window.addEventListener("focus", checkPendingTopups);
    return () => {
      cancelled = true;
      window.clearInterval(interval);
      window.removeEventListener("focus", checkPendingTopups);
    };
  }, [user]);

  return null;
}

