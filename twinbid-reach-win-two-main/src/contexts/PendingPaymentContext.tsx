import { createContext, useContext, useState, useCallback, useRef, type Dispatch, type ReactNode, type SetStateAction } from "react";
import type { PaymentChannel, TopupStatus } from "@/api/types";

export interface PendingPaymentData {
  amount: number;
  method: string;
  channel?: PaymentChannel;
  promo?: string;
  bonus?: number;
  /** Resolved promocode id captured at apply-time to avoid re-lookup at submit. */
  promocode_id?: string | null;
  /** Actual bonus amount in $ (from backend tx.bonus_amount on rehydrate). Source of truth for the notification total. */
  bonus_amount?: number;
  /** Backend row id (`transaction.id`). Used in every TwinBid transaction URL. */
  transactionRowId?: string | null;
  total_balance_increase?: number;
  status?: TopupStatus;
  payment_url?: string | null;
  provider_status?: string | null;
  amount_paid?: number | null;
  amount_credited?: number | null;
  credited_at?: string | null;
}

interface PendingPaymentContextType {
  pendingPayment: PendingPaymentData | null;
  setPendingPayment: Dispatch<SetStateAction<PendingPaymentData | null>>;
  isDialogOpen: boolean;
  openDialog: () => void;
  closeDialog: () => void;
  openPayment: (payment: PendingPaymentData) => void;
  restorePaymentAfterPassimPay: () => void;
  // Notify Balance page to refresh history after submission
  registerRefreshHandler: (fn: () => void) => void;
  triggerRefresh: () => void;
}

const PendingPaymentContext = createContext<PendingPaymentContextType | null>(null);

export function PendingPaymentProvider({ children }: { children: ReactNode }) {
  const [pendingPayment, setPendingPayment] = useState<PendingPaymentData | null>(null);
  const [isDialogOpen, setDialogOpen] = useState(false);
  const refreshRef = useRef<() => void>(() => {});
  const savedStaticPaymentRef = useRef<PendingPaymentData | null>(null);

  const openDialog = useCallback(() => setDialogOpen(true), []);
  const closeDialog = useCallback(() => setDialogOpen(false), []);
  const openPayment = useCallback((payment: PendingPaymentData) => {
    setPendingPayment(current => {
      if (payment.channel === "passimpay_invoice" && current && current.channel !== "passimpay_invoice") {
        savedStaticPaymentRef.current = current;
      }
      return payment;
    });
    setDialogOpen(true);
  }, []);
  const restorePaymentAfterPassimPay = useCallback(() => {
    setPendingPayment(current => {
      if (current?.channel !== "passimpay_invoice") return current;
      const savedStaticPayment = savedStaticPaymentRef.current;
      savedStaticPaymentRef.current = null;
      return savedStaticPayment;
    });
  }, []);

  const registerRefreshHandler = useCallback((fn: () => void) => {
    refreshRef.current = fn;
  }, []);
  const triggerRefresh = useCallback(() => {
    refreshRef.current?.();
  }, []);

  return (
    <PendingPaymentContext.Provider value={{
      pendingPayment, setPendingPayment,
      isDialogOpen, openDialog, closeDialog,
      openPayment, restorePaymentAfterPassimPay,
      registerRefreshHandler, triggerRefresh,
    }}>
      {children}
    </PendingPaymentContext.Provider>
  );
}

export function usePendingPayment() {
  const ctx = useContext(PendingPaymentContext);
  if (!ctx) throw new Error("usePendingPayment must be used within PendingPaymentProvider");
  return ctx;
}
