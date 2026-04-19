export type ApiUser = { id: string; email: string };
export type ApiSession = { access_token: string; user: ApiUser };

export type ProfileDto = {
  id: string;
  email: string | null;
  fullName: string | null;
  telegram: string | null;
  timezone: string | null;
  balance: number;
  balanceThreshold: number;
  notifyCampaignStatus: boolean;
  notifyLowBalance: boolean;
};

export type CampaignDto = {
  id: string;
  userId: string;
  name: string;
  status: "active" | "paused" | "draft" | "completed" | "moderation";
  format: string;
  formatKey: string;
  budget: number;
  dailyBudget: number | null;
  spent: number;
  impressions: number;
  clicks: number;
  ctr: number;
  pricingModel: "cpm" | "cpc";
  priceValue: number;
  trafficQuality: "common" | "high" | "ultra";
  startDate: string;
  endDate: string;
  creatives: Array<{
    id: string;
    name?: string;
    url: string;
    imageUrl?: string;
    imageFileName?: string;
    storagePath?: string;
    title?: string;
    description?: string;
  }>;
  targeting: Record<string, { mode: "none" | "white" | "black"; items: string[] }>;
  evenSpend: boolean;
  bannerSize?: string;
  brandName?: string;
  trafficType: "mainstream" | "adult" | "mixed";
  verticals: string[];
  description?: string;
  createdAt: string;
};

export type TopupRequestDto = {
  id: string;
  userId: string;
  amount: number;
  created_at: string;
  payment_method: string;
  status: "pending" | "approved" | "rejected";
  promo_code: string | null;
  bonus_percent: number | null;
  tx_hash: string;
};

type DbUser = {
  id: string;
  email: string;
  password: string;
  fullName: string;
};

type PromoCode = {
  id: string;
  code: string;
  bonusPercent: number;
  isActive: boolean;
  expiresAt: string | null;
  maxUses: number | null;
  currentUses: number;
};

type DbSchema = {
  users: DbUser[];
  profiles: ProfileDto[];
  campaigns: CampaignDto[];
  topupRequests: TopupRequestDto[];
  promoCodes: PromoCode[];
  promoUsage: Array<{ id: string; userId: string; promoCodeId: string; usedAt: string }>;
  sessions: Array<{ token: string; userId: string }>;
};

const DB_KEY = "twinbid_api_stub_db_v1";
const TOKEN_KEY = "twinbid_api_stub_token";

const USE_API_STUBS = (import.meta.env.VITE_USE_API_STUBS ?? "true") !== "false";

function uid(prefix = "id") {
  return `${prefix}_${Math.random().toString(36).slice(2, 10)}_${Date.now().toString(36)}`;
}

function seedDb(): DbSchema {
  const userId = "user_demo_1";
  return {
    users: [{ id: userId, email: "demo@twinbid.local", password: "demo123", fullName: "Demo User" }],
    profiles: [{
      id: userId,
      email: "demo@twinbid.local",
      fullName: "Demo User",
      telegram: "@demo_user",
      timezone: "utc_3",
      balance: 500,
      balanceThreshold: 100,
      notifyCampaignStatus: true,
      notifyLowBalance: true,
    }],
    campaigns: [],
    topupRequests: [],
    promoCodes: [
      { id: "promo_welcome", code: "WELCOME10", bonusPercent: 10, isActive: true, expiresAt: null, maxUses: null, currentUses: 0 },
      { id: "promo_spring", code: "SPRING20", bonusPercent: 20, isActive: true, expiresAt: "2026-12-31T00:00:00.000Z", maxUses: 1000, currentUses: 0 },
    ],
    promoUsage: [],
    sessions: [],
  };
}

function readDb(): DbSchema {
  const raw = localStorage.getItem(DB_KEY);
  if (!raw) {
    const seeded = seedDb();
    localStorage.setItem(DB_KEY, JSON.stringify(seeded));
    return seeded;
  }
  return JSON.parse(raw) as DbSchema;
}

function writeDb(db: DbSchema) {
  localStorage.setItem(DB_KEY, JSON.stringify(db));
}

function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

function getCurrentUserFromToken(db: DbSchema): DbUser | null {
  const token = getToken();
  if (!token) return null;
  const session = db.sessions.find(s => s.token === token);
  if (!session) return null;
  return db.users.find(u => u.id === session.userId) ?? null;
}

function apiError(message: string) {
  return { message };
}

export async function getSession(): Promise<ApiSession | null> {
  if (!USE_API_STUBS) return null;
  const db = readDb();
  const token = getToken();
  if (!token) return null;
  const user = getCurrentUserFromToken(db);
  if (!user) return null;
  return { access_token: token, user: { id: user.id, email: user.email } };
}

export async function signUp(email: string, password: string, fullName?: string): Promise<{ error: any }> {
  if (!USE_API_STUBS) return { error: apiError("Real backend is not configured") };
  const db = readDb();
  const normalized = email.trim().toLowerCase();
  if (db.users.some(u => u.email.toLowerCase() === normalized)) return { error: apiError("User already exists") };
  const id = uid("user");
  db.users.push({ id, email: normalized, password, fullName: fullName || "" });
  db.profiles.push({
    id,
    email: normalized,
    fullName: fullName || "",
    telegram: null,
    timezone: "utc_3",
    balance: 0,
    balanceThreshold: 100,
    notifyCampaignStatus: true,
    notifyLowBalance: true,
  });
  const token = uid("token");
  db.sessions.push({ token, userId: id });
  writeDb(db);
  localStorage.setItem(TOKEN_KEY, token);
  return { error: null };
}

export async function signIn(email: string, password: string): Promise<{ error: any }> {
  if (!USE_API_STUBS) return { error: apiError("Real backend is not configured") };
  const db = readDb();
  const normalized = email.trim().toLowerCase();
  const user = db.users.find(u => u.email.toLowerCase() === normalized && u.password === password);
  if (!user) return { error: apiError("Invalid email or password") };
  const token = uid("token");
  db.sessions.push({ token, userId: user.id });
  writeDb(db);
  localStorage.setItem(TOKEN_KEY, token);
  return { error: null };
}

export async function signOut(): Promise<void> {
  if (!USE_API_STUBS) return;
  const db = readDb();
  const token = getToken();
  if (token) db.sessions = db.sessions.filter(s => s.token !== token);
  writeDb(db);
  localStorage.removeItem(TOKEN_KEY);
}

export async function updatePassword(currentPassword: string, newPassword: string): Promise<{ error: any }> {
  if (!USE_API_STUBS) return { error: apiError("Real backend is not configured") };
  const db = readDb();
  const user = getCurrentUserFromToken(db);
  if (!user) return { error: apiError("Unauthorized") };
  if (user.password !== currentPassword) return { error: apiError("Current password is incorrect") };
  user.password = newPassword;
  writeDb(db);
  return { error: null };
}

export async function getProfile(userId: string): Promise<ProfileDto | null> {
  if (!USE_API_STUBS) return null;
  const db = readDb();
  return db.profiles.find(p => p.id === userId) ?? null;
}

export async function updateProfile(userId: string, updates: Partial<Omit<ProfileDto, "id">>): Promise<{ error: any }> {
  if (!USE_API_STUBS) return { error: apiError("Real backend is not configured") };
  const db = readDb();
  const profile = db.profiles.find(p => p.id === userId);
  if (!profile) return { error: apiError("Profile not found") };
  Object.assign(profile, updates);
  writeDb(db);
  return { error: null };
}

export async function listCampaigns(userId: string): Promise<CampaignDto[]> {
  if (!USE_API_STUBS) return [];
  const db = readDb();
  return db.campaigns
    .filter(c => c.userId === userId)
    .sort((a, b) => (a.createdAt < b.createdAt ? 1 : -1));
}

export async function createCampaign(userId: string, campaign: Omit<CampaignDto, "id" | "userId" | "createdAt">): Promise<string> {
  const db = readDb();
  const id = uid("cmp");
  db.campaigns.push({ ...campaign, id, userId, createdAt: new Date().toISOString() });
  writeDb(db);
  return id;
}

export async function updateCampaign(id: string, updates: Partial<CampaignDto>): Promise<void> {
  const db = readDb();
  const campaign = db.campaigns.find(c => c.id === id);
  if (!campaign) return;
  Object.assign(campaign, updates);
  writeDb(db);
}

export async function deleteCampaign(id: string): Promise<void> {
  const db = readDb();
  db.campaigns = db.campaigns.filter(c => c.id !== id);
  writeDb(db);
}

export async function listTopupRequests(userId: string): Promise<TopupRequestDto[]> {
  const db = readDb();
  return db.topupRequests.filter(t => t.userId === userId).sort((a, b) => (a.created_at < b.created_at ? 1 : -1));
}

export async function validatePromo(code: string): Promise<{ valid: boolean; bonusPercent?: number; promoId?: string }> {
  const db = readDb();
  const promo = db.promoCodes.find(p => p.code === code.toUpperCase() && p.isActive);
  if (!promo) return { valid: false };
  if (promo.expiresAt && new Date(promo.expiresAt) < new Date()) return { valid: false };
  if (promo.maxUses && promo.currentUses >= promo.maxUses) return { valid: false };
  return { valid: true, bonusPercent: promo.bonusPercent, promoId: promo.id };
}

export async function createTopupRequest(params: {
  userId: string;
  amount: number;
  paymentMethod: string;
  txHash: string;
  promoCode?: string;
  bonusPercent?: number;
}): Promise<{ error: any }> {
  const db = readDb();
  db.topupRequests.push({
    id: uid("topup"),
    userId: params.userId,
    amount: params.amount,
    payment_method: params.paymentMethod,
    tx_hash: params.txHash,
    status: "pending",
    promo_code: params.promoCode ?? null,
    bonus_percent: params.bonusPercent ?? null,
    created_at: new Date().toISOString(),
  });

  if (params.promoCode) {
    const promo = db.promoCodes.find(p => p.code === params.promoCode.toUpperCase());
    if (promo) {
      promo.currentUses += 1;
      db.promoUsage.push({ id: uid("usage"), userId: params.userId, promoCodeId: promo.id, usedAt: new Date().toISOString() });
    }
  }

  writeDb(db);
  return { error: null };
}
