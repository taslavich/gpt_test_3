import { describe, expect, it } from "vitest";
import { ApiError } from "@/api";
import { isEmailConfirmationRequired } from "@/lib/authErrors";

describe("email confirmation login errors", () => {
  it("recognizes the current backend 403 response", () => {
    expect(isEmailConfirmationRequired(new ApiError(403, "Forbidden"))).toBe(true);
  });

  it("recognizes confirmation errors returned inside a successful HTTP envelope", () => {
    expect(isEmailConfirmationRequired(new ApiError(200, "email is not confirmed"))).toBe(true);
  });

  it("recognizes a backend error code regardless of its status", () => {
    expect(isEmailConfirmationRequired(new ApiError(401, "Unauthorized", "EMAIL_NOT_VERIFIED"))).toBe(true);
  });

  it("does not confuse invalid credentials with an unconfirmed email", () => {
    expect(isEmailConfirmationRequired(new ApiError(401, "invalid email or password"))).toBe(false);
  });
});
