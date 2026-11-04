import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "./AuthContext";
import { usePublisherPayout } from "./usePublisherPayout";
import {
  publisherApi,
  type PublisherCodeDTO,
  type CreatePublisherCode,
  type PatchPublisherCode,
  type PublisherPayout,
  type CreatePublisherPayout,
} from "@/api/publisher";
// The public landing page has its own format illustrations.
export type ZoneFormat =
  | "banner"
  | "popunder"
  | "inpage_push"
  | "native"
  | "video_popup"
  | "instream"
  | "outstream";
export type PublisherAdCode = PublisherCodeDTO;
export type TrafficType = PublisherCodeDTO["traffic_type"];
export type PayoutStatus = PublisherPayout["status"];
interface PublisherContextValue {
  scope: string;
  adCodes: PublisherCodeDTO[];
  publicScriptUrl?: string;
  directLinkAvailable: boolean;
  codePage: { limit: number; offset: number };
  setCodePage: (page: { limit: number; offset: number }) => void;
  loading: boolean;
  error: unknown;
  refetch: () => Promise<unknown>;
  mutationPending: boolean;
  createAdCode: (input: CreatePublisherCode) => Promise<PublisherCodeDTO>;
  updateAdCode: (
    id: string,
    patch: PatchPublisherCode,
  ) => Promise<PublisherCodeDTO>;
  duplicateAdCode: (id: string) => Promise<PublisherCodeDTO>;
  deleteAdCode: (id: string) => Promise<{ archived: boolean }>;
  createPayout: (
    input: Omit<CreatePublisherPayout, "request_id">,
  ) => Promise<PublisherPayout>;
  payoutIntent: CreatePublisherPayout | null;
  payoutBusy: boolean;
  refreshPublisher: () => Promise<void>;
}
const PublisherContext = createContext<PublisherContextValue | null>(null);
export function PublisherProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth(),
    client = useQueryClient();
  const scope = user ? user.id + ":" + user.email.trim().toLowerCase() : "";
  const key = useMemo(() => ["publisher", scope], [scope]);
  const [codePage, setCodePage] = useState({ limit: 50, offset: 0 });
  const codes = useQuery({
    queryKey: [...key, "codes", codePage],
    queryFn: ({ signal }) => publisherApi.codes(codePage, signal),
    enabled: !!scope,
    retry: false,
  });
  const refreshPublisher = useCallback(async () => {
    await client.invalidateQueries({ queryKey: key });
  }, [client, key]);
  const create = useMutation({
    mutationFn: publisherApi.createCode,
    onSuccess: refreshPublisher,
  });
  const patch = useMutation({
    mutationFn: ({ id, body }: { id: string; body: PatchPublisherCode }) =>
      publisherApi.patchCode(id, body),
    onSuccess: refreshPublisher,
  });
  const duplicate = useMutation({
    mutationFn: publisherApi.duplicateCode,
    onSuccess: refreshPublisher,
  });
  const archive = useMutation({
    mutationFn: publisherApi.archiveCode,
    onSuccess: refreshPublisher,
  });
  // Email is stable across sign-in and /session hydration; AuthProvider uses
  // a placeholder id immediately after login. The journal never contains tokens.
  const account = user?.email.trim().toLowerCase() || "";
  const { payoutIntent, payoutBusy, createPayout } = usePublisherPayout(
    account,
    scope,
    refreshPublisher,
  );
  useEffect(() => {
    setCodePage({ limit: 50, offset: 0 });
    return () => {
      void client.cancelQueries({ queryKey: key });
      client.removeQueries({ queryKey: key });
    };
  }, [client, key]);
  return (
    <PublisherContext.Provider
      key={scope}
      value={{
        scope,
        adCodes: codes.data?.items || [],
        publicScriptUrl: codes.data?.public_script_url,
        directLinkAvailable: codes.data?.supported_formats?.includes("direct_link") ?? false,
        codePage,
        setCodePage,
        loading: codes.isPending,
        error: codes.error,
        refetch: codes.refetch,
        mutationPending:
          create.isPending ||
          patch.isPending ||
          duplicate.isPending ||
          archive.isPending,
        createAdCode: create.mutateAsync,
        updateAdCode: (id, body) => patch.mutateAsync({ id, body }),
        duplicateAdCode: duplicate.mutateAsync,
        deleteAdCode: archive.mutateAsync,
        createPayout,
        payoutIntent,
        payoutBusy,
        refreshPublisher,
      }}
    >
      {children}
    </PublisherContext.Provider>
  );
}
export function usePublisher() {
  const value = useContext(PublisherContext);
  if (!value)
    throw new Error("usePublisher must be used within PublisherProvider");
  return value;
}
export function usePublisherResource<T>(
  key: readonly unknown[],
  load: (signal: AbortSignal) => Promise<T>,
  enabled = true,
) {
  const { scope } = usePublisher();
  return useQuery({
    queryKey: ["publisher", scope, ...key],
    queryFn: ({ signal }) => load(signal),
    enabled: !!scope && enabled,
    retry: false,
    refetchOnWindowFocus: false,
    refetchInterval: ["balance", "overview", "sync-health"].includes(
      String(key[0]),
    )
      ? 30000
      : ["diagnostics", "zone-diagnostics"].includes(String(key[0]))
        ? 60000
        : false,
  });
}
