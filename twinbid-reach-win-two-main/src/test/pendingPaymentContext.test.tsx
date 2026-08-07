import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { PendingPaymentProvider, usePendingPayment } from "@/contexts/PendingPaymentContext";

function Harness() {
  const { pendingPayment, openPayment, restorePaymentAfterPassimPay } = usePendingPayment();
  return (
    <>
      <div data-testid="channel">{pendingPayment?.channel || "none"}</div>
      <button onClick={() => openPayment({ amount: 100, method: "usdt", channel: "static_wallet" })}>
        static
      </button>
      <button onClick={() => openPayment({ amount: 100, method: "passimpay", channel: "passimpay_invoice" })}>
        passim
      </button>
      <button onClick={restorePaymentAfterPassimPay}>restore</button>
    </>
  );
}

describe("pending payment context", () => {
  it("restores an unfinished static payment after viewing PassimPay", () => {
    render(<PendingPaymentProvider><Harness /></PendingPaymentProvider>);

    fireEvent.click(screen.getByText("static"));
    expect(screen.getByTestId("channel")).toHaveTextContent("static_wallet");

    fireEvent.click(screen.getByText("passim"));
    expect(screen.getByTestId("channel")).toHaveTextContent("passimpay_invoice");

    fireEvent.click(screen.getByText("restore"));
    expect(screen.getByTestId("channel")).toHaveTextContent("static_wallet");
  });
});
