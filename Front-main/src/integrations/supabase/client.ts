import {
  createTopupRequest,
  listTopupRequests,
  updatePassword,
  validatePromo,
} from "@/lib/api";

type Row = Record<string, any>;

class SupabaseCompatQuery {
  private filters: Record<string, any> = {};
  private selected = "*";

  constructor(private table: string) {}

  select(columns: string) {
    this.selected = columns;
    return this;
  }

  eq(column: string, value: any) {
    this.filters[column] = value;
    return this;
  }

  order(_column: string, _opts?: { ascending?: boolean }) {
    return this;
  }

  async single() {
    const { data } = await this.execSelect();
    return { data: Array.isArray(data) ? data[0] ?? null : data ?? null, error: null };
  }

  async insert(payload: Row | Row[]) {
    if (this.table === "topup_requests") {
      const rows = Array.isArray(payload) ? payload : [payload];
      for (const row of rows) {
        await createTopupRequest({
          userId: row.user_id,
          amount: Number(row.amount) || 0,
          paymentMethod: row.payment_method || "unknown",
          txHash: row.tx_hash || "stub_tx_hash",
          promoCode: row.promo_code || undefined,
          bonusPercent: Number(row.bonus_percent) || 0,
        });
      }
      return { data: null, error: null };
    }

    // promo_usage and unknown tables are accepted as no-op for compatibility.
    return { data: null, error: null };
  }

  async update(_payload: Row) {
    return { data: null, error: null };
  }

  async delete() {
    return { data: null, error: null };
  }

  async then(resolve: (value: any) => void, reject?: (reason?: any) => void) {
    try {
      resolve(await this.execSelect());
    } catch (err) {
      reject?.(err);
    }
  }

  private async execSelect() {
    if (this.table === "topup_requests") {
      const userId = this.filters.user_id;
      const data = userId ? await listTopupRequests(userId) : [];
      return { data, error: null };
    }

    if (this.table === "promo_codes") {
      const code = (this.filters.code || "").toUpperCase();
      const result = await validatePromo(code);
      if (!result.valid) return { data: null, error: { message: "Promo not found" } };
      return {
        data: {
          id: result.promoId,
          code,
          bonus_percent: result.bonusPercent ?? 0,
          is_active: true,
          expires_at: null,
          max_uses: null,
          current_uses: 0,
        },
        error: null,
      };
    }

    return { data: this.selected === "*" ? [] : null, error: null };
  }
}

export const supabase = {
  auth: {
    async updateUser(payload: { password?: string }) {
      if (!payload.password) return { data: null, error: { message: "Password is required" } };
      const result = await updatePassword("", payload.password);
      return { data: null, error: result.error };
    },
  },
  from(table: string) {
    return new SupabaseCompatQuery(table);
  },
};

