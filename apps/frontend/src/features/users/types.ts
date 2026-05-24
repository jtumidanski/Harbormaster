export type UserStatus = "enabled" | "disabled";

export type TemplateRef = {
  name: string;
  params?: Record<string, string> | null;
};

export type User = {
  access_key: string;
  status: UserStatus;
  attached_templates: TemplateRef[];
  other_policies: string[];
};

export type UserResource = {
  type: "users";
  id: string;
  attributes: User;
};

export type UserCollectionResponse = {
  data: UserResource[];
};

export type UserSingleResponse = {
  data: UserResource;
};

export type CreateUserResponseAttrs = User & { secret_key: string };

export type CreateUserResponse = {
  data: {
    type: "users";
    id: string;
    attributes: CreateUserResponseAttrs;
  };
};
