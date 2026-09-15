export type OrganizationAssigneeType = "agent" | "squad";
export type OrganizationResolutionState = "resolved" | "unresolved" | "invalid" | "blocked" | string;

export interface OrganizationBinding {
  type: OrganizationAssigneeType;
  id: string;
  name?: string;
  runtime?: string;
  skills: string[];
  permissions: string[];
  availability?: string;
}

export interface OrganizationResolution {
  state: OrganizationResolutionState;
  source?: "project" | "workspace" | "template" | string;
  reason?: string;
}

export interface OrganizationRole {
  key: string;
  name: string;
  description?: string;
  responsibilities: string[];
  required_skills: string[];
  recommended_skills: string[];
  capabilities: string[];
  binding?: OrganizationBinding;
  resolution: OrganizationResolution;
}

export interface AgentOrganization {
  workspace_id: string;
  project_id?: string;
  roles: OrganizationRole[];
}

export interface PutOrganizationRoleBindingRequest {
  project_id?: string;
  assignee_type: OrganizationAssigneeType;
  assignee_id: string;
}
