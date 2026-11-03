import { useCallback, useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { ApiError } from "@/api/http";
import {
  publisherApi,
  type CreatePublisherPayout,
  type PublisherPayout,
} from "@/api/publisher";
import {
  clearPayoutOperation,
  isPayoutRequestId,
  payoutMatchesRequest,
  payoutOperationKey,
  readPayoutOperation,
  samePayoutInput,
  withPayoutLock,
  writePayoutOperation,
  type PayoutOperation,
} from "@/lib/publisherPayoutOperation";

export function usePublisherPayout(
  account: string,
  scope: string,
  refreshPublisher: () => Promise<void>,
) {
  const client = useQueryClient();
  const owner = useRef({ account, scope });
  if (owner.current.account !== account || owner.current.scope !== scope)
    owner.current = { account, scope };
  const mounted = useRef(false);
  const [state, setState] = useState<{
    account: string;
    operation: PayoutOperation | null;
    busy: boolean;
  }>({ account, operation: null, busy: !!account });
  const observed = useRef<{
    account: string;
    operation: PayoutOperation | null;
  }>({ account: "", operation: null });
  const flight = useRef<{
    account: string;
    input: Omit<CreatePublisherPayout, "request_id">;
    promise: Promise<PublisherPayout>;
  } | null>(null);
  const show = useCallback(
    (
      operation: PayoutOperation | null,
      busy: boolean,
      token: typeof owner.current,
    ) => {
      if (!mounted.current || owner.current !== token) return;
      observed.current = { account, operation };
      setState({ account, operation, busy });
    },
    [account],
  );
  const history = useCallback(
    async (operation: PayoutOperation) => {
      const page = await client.fetchQuery({
        queryKey: ["publisher", scope, "payouts", { limit: 50, offset: 0 }],
        queryFn: ({ signal }) =>
          publisherApi.payouts({ limit: 50, offset: 0 }, signal),
        retry: false,
        staleTime: 0,
      });
      // The current GET DTO has no request_id. Equal amount/wallet/time (or an
      // absent row on this page) is not proof. Only a previously received ID is exact.
      return operation.payout_id
        ? page.items.find(
            (p) =>
              p.id === operation.payout_id &&
              payoutMatchesRequest(p, operation.request),
          )
        : undefined;
    },
    [client, scope],
  );

  useEffect(() => {
    mounted.current = true;
    const token = owner.current;
    let live = true;
    const sync = (busy = false) => {
      if (!live || owner.current !== token) return;
      try {
        show(account ? readPayoutOperation(account) : null, busy, token);
      } catch {
        show(null, false, token);
      } // createPayout reads again and fails closed before POST.
    };
    const storage = (event: StorageEvent) => {
      if (event.key === payoutOperationKey(account) || event.key === null) {
        // Keep this tab's unresolved intent until its own exact confirmation.
        // Clearing it underneath an open retry dialog would turn a stale click
        // into a new UUID after another tab has completed the same operation.
        const previous =
          observed.current.account === account
            ? observed.current.operation
            : null;
        if (previous)
          show(
            previous,
            !!flight.current && flight.current.account === account,
            token,
          );
        else sync();
        if (account) void refreshPublisher();
      }
    };
    window.addEventListener("storage", storage);
    sync(!!account);
    if (account) {
      let saved: PayoutOperation | null = null;
      try {
        saved = readPayoutOperation(account);
      } catch {
        /* Remain blocked on submission. */
      }
      if (saved) {
        void withPayoutLock(account, async () => {
          if (!live || owner.current !== token) return;
          const latest = readPayoutOperation(account);
          if (!latest) return;
          const confirmed = await history(latest);
          if (confirmed) {
            clearPayoutOperation(account, latest.request.request_id);
            if (live && owner.current === token) await refreshPublisher();
          }
        })
          .catch(() => {
            /* An unavailable or ambiguous history preserves the operation. */
          })
          .finally(() => sync());
      } else sync();
    }
    return () => {
      live = false;
      mounted.current = false;
      window.removeEventListener("storage", storage);
    };
  }, [account, scope, history, refreshPublisher, show]);

  const createPayout = useCallback(
    (input: Omit<CreatePublisherPayout, "request_id">) => {
      if (!account)
        return Promise.reject(
          new ApiError(401, "Sign in before requesting a payout"),
        );
      if (owner.current.account !== account || owner.current.scope !== scope)
        return Promise.reject(
          new ApiError(
            409,
            "The signed-in account changed before payout submission",
            "PAYOUT_OWNER_CHANGED",
          ),
        );
      const running = flight.current;
      if (running?.account === account) {
        return samePayoutInput(running.input, input)
          ? running.promise
          : Promise.reject(
              new ApiError(
                409,
                "Resolve the previous payout request before changing its details",
                "PAYOUT_PENDING",
              ),
            );
      }
      const token = owner.current;
      const previous =
        observed.current.account === account
          ? observed.current.operation
          : null;
      // The unchanged payout page passes its restored intent to this method.
      // Retain that key even when another tab has just cleared its journal.
      const suppliedId: unknown = (input as Partial<CreatePublisherPayout>)
        .request_id;
      if (suppliedId !== undefined && !isPayoutRequestId(suppliedId))
        return Promise.reject(new ApiError(400, "Invalid payout request_id"));
      const payload = {
        amount: input.amount,
        method: input.method,
        wallet: input.wallet,
      };
      show(previous, true, token);
      const assertOwner = () => {
        if (!mounted.current || owner.current !== token)
          throw new ApiError(
            409,
            "The signed-in account changed before payout submission",
            "PAYOUT_OWNER_CHANGED",
          );
      };
      const attempt = withPayoutLock(account, async () => {
        assertOwner();
        let operation = readPayoutOperation(account);
        if (!operation && (previous || suppliedId))
          operation = previous || {
            version: 1,
            request: { ...payload, request_id: suppliedId as string },
            attempted: true,
          };
        if (
          operation &&
          (!samePayoutInput(operation.request, payload) ||
            (suppliedId && suppliedId !== operation.request.request_id))
        )
          throw new ApiError(
            409,
            "Resolve the previous payout request before changing its details",
            "PAYOUT_PENDING",
          );
        if (operation) {
          show(operation, true, token);
          let confirmed: PublisherPayout | undefined;
          try {
            confirmed = await history(operation);
          } catch {
            /* Safe manual retry still uses the old request_id. */
          }
          if (confirmed) {
            clearPayoutOperation(account, operation.request.request_id);
            if (owner.current === token && mounted.current)
              await refreshPublisher();
            return confirmed;
          }
        } else {
          operation = {
            version: 1,
            request: { ...payload, request_id: crypto.randomUUID() },
            attempted: false,
          };
        }
        // Durably mark a possible transmission before invoking the authenticated client.
        // A reload during POST must never turn the action back into a fresh UUID.
        assertOwner();
        const firstAttempt = !operation.attempted;
        operation = { ...operation, attempted: true };
        writePayoutOperation(account, operation);
        show(operation, true, token);
        let value: PublisherPayout;
        try {
          value = await publisherApi.createPayout(operation.request);
        } catch (error) {
          // Only a first, definitively refused POST permits a new corrected action.
          // A 4xx on a retry cannot disprove a commit before an earlier lost response.
          if (
            firstAttempt &&
            error instanceof ApiError &&
            error.status >= 400 &&
            error.status < 500
          )
            clearPayoutOperation(account, operation.request.request_id);
          throw error;
        }
        if (
          !payoutMatchesRequest(value, operation.request) ||
          (operation.payout_id && operation.payout_id !== value.id)
        )
          throw new ApiError(
            409,
            "The payout response does not match the saved request",
            "PAYOUT_RESPONSE",
          );
        // Save the exact server ID before cleanup, so GET can reconcile even when
        // removing storage fails or the browser closes between these two writes.
        writePayoutOperation(account, { ...operation, payout_id: value.id });
        clearPayoutOperation(account, operation.request.request_id);
        if (owner.current === token && mounted.current)
          await refreshPublisher();
        return value;
      }).finally(() => {
        if (flight.current?.promise === attempt) flight.current = null;
        if (owner.current === token && mounted.current) {
          try {
            show(readPayoutOperation(account), false, token);
          } catch {
            show(previous, false, token);
          }
        }
      });
      flight.current = { account, input: payload, promise: attempt };
      return attempt;
    },
    [account, scope, history, refreshPublisher, show],
  );
  return {
    createPayout,
    payoutIntent:
      state.account === account ? state.operation?.request || null : null,
    payoutBusy: state.account === account ? state.busy : !!account,
  };
}
