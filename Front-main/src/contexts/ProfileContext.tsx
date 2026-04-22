import { createContext, useContext, useState, useCallback, useEffect, type ReactNode } from "react";
import { useAuth } from "@/contexts/AuthContext";
import { getProfile as apiGetProfile, updateProfile as apiUpdateProfile } from "@/lib/api";

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

export function ProfileProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth();
  const [profile, setProfile] = useState<Profile | null>(null);
  const [loading, setLoading] = useState(true);

  const fetchProfile = useCallback(async () => {
    if (!user) {
      setProfile(null);
      setLoading(false);
      return;
    }
    setLoading(true);
    const data = await apiGetProfile(user.id);
    setProfile(data);
    setLoading(false);
  }, [user]);

  useEffect(() => {
    fetchProfile();
  }, [fetchProfile]);

  const updateProfile = useCallback(
    async (updates: Partial<Omit<Profile, "id">>) => {
      if (!user) return;
      const { error } = await apiUpdateProfile(user.id, updates);
      if (error) throw error;
      await fetchProfile();
    },
    [user, fetchProfile],
  );

  return <ProfileContext.Provider value={{ profile, loading, refetch: fetchProfile, updateProfile }}>{children}</ProfileContext.Provider>;
}

export function useProfile() {
  const ctx = useContext(ProfileContext);
  if (!ctx) throw new Error("useProfile must be used within ProfileProvider");
  return ctx;
}
