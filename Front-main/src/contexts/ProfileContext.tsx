import { createContext, useContext, useState, useCallback, useEffect, type ReactNode } from "react";
import { supabase } from "@/integrations/supabase/client";
import { useAuth } from "@/contexts/AuthContext";

export interface Profile {
  id: string;
  email: string | null;
  fullName: string | null;
  telegram: string | null;
  timezone: string | null;
  balance: number;
  balanceThreshold: number;
  notifyCampaignStatus: boolean;
  notifyLowBalance: boolean;
}

interface ProfileContextType {
  profile: Profile | null;
  loading: boolean;
  refetch: () => Promise<void>;
  updateProfile: (updates: Partial<Omit<Profile, "id">>) => Promise<void>;
}

const ProfileContext = createContext<ProfileContextType | null>(null);

function mapProfileFromDb(row: any): Profile {
  return {
    id: row.id,
    email: row.email,
    fullName: row.full_name,
    telegram: row.telegram,
    timezone: row.timezone,
    balance: Number(row.balance) || 0,
    balanceThreshold: Number(row.balance_threshold) || 100,
    notifyCampaignStatus: row.notify_campaign_status ?? true,
    notifyLowBalance: row.notify_low_balance ?? true,
  };
}

export function ProfileProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth();
  const [profile, setProfile] = useState<Profile | null>(null);
  const [loading, setLoading] = useState(true);

  const fetchProfile = useCallback(async () => {
    if (!user) { setProfile(null); setLoading(false); return; }
    setLoading(true);
    const { data, error } = await supabase
      .from("profiles")
      .select("*")
      .eq("id", user.id)
      .single();
    if (error) { console.error("Profile fetch error:", error); setLoading(false); return; }
    setProfile(mapProfileFromDb(data));
    setLoading(false);
  }, [user]);

  useEffect(() => { fetchProfile(); }, [fetchProfile]);

  const updateProfile = useCallback(async (updates: Partial<Omit<Profile, "id">>) => {
    if (!user) return;
    const dbUpdates: any = {};
    if (updates.fullName !== undefined) dbUpdates.full_name = updates.fullName;
    if (updates.email !== undefined) dbUpdates.email = updates.email;
    if (updates.telegram !== undefined) dbUpdates.telegram = updates.telegram;
    if (updates.timezone !== undefined) dbUpdates.timezone = updates.timezone;
    if (updates.balanceThreshold !== undefined) dbUpdates.balance_threshold = updates.balanceThreshold;
    if (updates.notifyCampaignStatus !== undefined) dbUpdates.notify_campaign_status = updates.notifyCampaignStatus;
    if (updates.notifyLowBalance !== undefined) dbUpdates.notify_low_balance = updates.notifyLowBalance;

    const { error } = await supabase
      .from("profiles")
      .update(dbUpdates)
      .eq("id", user.id);
    if (error) throw error;
    await fetchProfile();
  }, [user, fetchProfile]);

  return (
    <ProfileContext.Provider value={{ profile, loading, refetch: fetchProfile, updateProfile }}>
      {children}
    </ProfileContext.Provider>
  );
}

export function useProfile() {
  const ctx = useContext(ProfileContext);
  if (!ctx) throw new Error("useProfile must be used within ProfileProvider");
  return ctx;
}
