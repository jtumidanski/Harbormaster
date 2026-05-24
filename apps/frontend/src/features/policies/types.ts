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
