import { z } from "zod";

export const adminSchema = z
  .object({
    username: z
      .string()
      .min(3, "must be at least 3 characters")
      .max(64)
      .regex(/^[a-z0-9._-]+$/, "lowercase letters, digits, dot, underscore, hyphen"),
    password: z.string().min(12, "must be at least 12 characters"),
    passwordConfirm: z.string(),
  })
  .refine((v) => v.password === v.passwordConfirm, {
    path: ["passwordConfirm"],
    message: "passwords must match",
  });

export const minioSchema = z.object({
  fromMcAlias: z.string().optional(),
  endpointUrl: z.string().url(),
  accessKey: z.string().min(1),
  secretKey: z.string().min(1),
  tlsSkipVerify: z.boolean().default(false),
  customCaPem: z.string().optional(),
});

export type AdminInput = z.infer<typeof adminSchema>;
export type MinIOInput = z.infer<typeof minioSchema>;
