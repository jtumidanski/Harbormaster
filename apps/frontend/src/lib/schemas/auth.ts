import { z } from "zod";

export const loginSchema = z.object({
  username: z.string().min(1, "required"),
  password: z.string().min(1, "required"),
});
export type LoginInput = z.infer<typeof loginSchema>;

export const changePasswordSchema = z
  .object({
    currentPassword: z.string().min(1, "required"),
    newPassword: z.string().min(12, "must be at least 12 characters"),
    newPasswordConfirm: z.string(),
  })
  .refine((v) => v.newPassword === v.newPasswordConfirm, {
    path: ["newPasswordConfirm"],
    message: "passwords must match",
  });
export type ChangePasswordInput = z.infer<typeof changePasswordSchema>;
