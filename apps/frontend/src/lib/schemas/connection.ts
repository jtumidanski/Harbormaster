import { z } from "zod";

export const connectionSchema = z.object({
  endpointUrl: z.string().url(),
  accessKey: z.string().min(1),
  secretKey: z.string().min(1),
  tlsSkipVerify: z.boolean().default(false),
  customCaPem: z.string().optional(),
});
export type ConnectionInput = z.infer<typeof connectionSchema>;
