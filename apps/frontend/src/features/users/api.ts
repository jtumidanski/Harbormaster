import { api } from "@/lib/api/client";
import type {
  CreateUserResponse,
  CreateUserResponseAttrs,
  TemplateRef,
  User,
  UserCollectionResponse,
  UserSingleResponse,
} from "./types";

export async function listUsers(): Promise<User[]> {
  const res = await api.get<UserCollectionResponse>("/api/v1/users");
  return res.data.map((d) => d.attributes);
}

export async function getUser(accessKey: string): Promise<User> {
  const res = await api.get<UserSingleResponse>(`/api/v1/users/${encodeURIComponent(accessKey)}`);
  return res.data.attributes;
}

export type CreateUserInput = {
  access_key: string;
  templates: TemplateRef[];
};

export async function createUser(input: CreateUserInput): Promise<CreateUserResponseAttrs> {
  const res = await api.post<CreateUserResponse>("/api/v1/users", {
    data: {
      type: "users",
      attributes: input,
    },
  });
  return res.data.attributes;
}

export async function setUserStatus(accessKey: string, enabled: boolean): Promise<void> {
  await api.put<void>(`/api/v1/users/${encodeURIComponent(accessKey)}/status`, { enabled });
}

export async function deleteUser(accessKey: string, confirmAccessKey: string): Promise<void> {
  await api.delete<void>(`/api/v1/users/${encodeURIComponent(accessKey)}`, {
    confirm_access_key: confirmAccessKey,
  });
}

export async function updateUserPolicies(
  accessKey: string,
  templates: TemplateRef[],
  policies: string[] = [],
): Promise<void> {
  await api.put<void>(`/api/v1/users/${encodeURIComponent(accessKey)}/policies`, {
    templates,
    policies,
  });
}
