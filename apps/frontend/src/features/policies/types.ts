export type PolicyOrigin = "minio-builtin" | "harbormaster-template" | "custom";

export type Policy = {
  name: string;
  origin: PolicyOrigin;
  editable: boolean;
  statement_summary: string;
};

export type PolicyDetail = Policy & {
  document: unknown;
};

export type PolicyResource = {
  type: "policies";
  id: string;
  attributes: Policy;
};

export type PolicyDetailResource = {
  type: "policies";
  id: string;
  attributes: PolicyDetail;
};

export type PolicyCollectionResponse = {
  data: PolicyResource[];
};

export type PolicySingleResponse = {
  data: PolicyDetailResource;
};

export type PolicyParamSchema = {
  type: "object";
  required?: string[];
  properties?: Record<
    string,
    {
      type: string;
      minLength?: number;
      maxLength?: number;
    }
  >;
};

export type PolicyTemplate = {
  name: string;
  description: string;
  params_schema: PolicyParamSchema | null;
};

export type PolicyTemplateResource = {
  type: "policy_templates";
  id: string;
  attributes: PolicyTemplate;
};

export type PolicyTemplateCollectionResponse = {
  data: PolicyTemplateResource[];
};
