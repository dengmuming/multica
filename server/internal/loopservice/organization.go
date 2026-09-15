package loopservice

import (
	"context"
	"errors"
	"strings"
)

// AgentOrganization is the product-facing engineering organization projection.
// It references canonical Multica workforce identities; it does not own them.
type AgentOrganization struct {
	WorkspaceID string             `json:"workspace_id"`
	ProjectID   string             `json:"project_id,omitempty"`
	Roles       []OrganizationRole `json:"roles"`
}

type OrganizationRole struct {
	Key               string                `json:"key"`
	Name              string                `json:"name"`
	Description       string                `json:"description,omitempty"`
	Responsibilities  []string              `json:"responsibilities"`
	RequiredSkills    []string              `json:"required_skills"`
	RecommendedSkills []string              `json:"recommended_skills"`
	Capabilities      []string              `json:"capabilities"`
	Binding           *OrganizationBinding  `json:"binding,omitempty"`
	Resolution        OrganizationResolution `json:"resolution"`
}

type OrganizationBinding struct {
	Type         string   `json:"type"`
	ID           string   `json:"id"`
	Name         string   `json:"name,omitempty"`
	Runtime      string   `json:"runtime,omitempty"`
	Skills       []string `json:"skills"`
	Permissions  []string `json:"permissions"`
	Availability string   `json:"availability,omitempty"`
}

type OrganizationResolution struct {
	State  string `json:"state"`
	Source string `json:"source,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type OrganizationReader interface {
	GetOrganization(ctx context.Context, workspaceID, projectID string) (*AgentOrganization, error)
}

type RoleBindingResolver interface {
	ResolveRoleBinding(ctx context.Context, workspaceID, projectID, roleKey string) (*OrganizationBinding, error)
}

// OrganizationCatalog provides stable product role semantics while execution
// remains backed by Multica Agent/Squad/Skill/Runtime primitives.
func OrganizationCatalog() []OrganizationRole {
	return []OrganizationRole{
		role("product", "Product", "Own intent, scope and acceptance criteria", []string{"requirements", "acceptance"}),
		role("architect", "Architect", "Own architecture and implementation decomposition", []string{"architecture", "planning"}),
		role("backend", "Backend", "Own API, service and data implementation", []string{"implementation", "api"}),
		role("frontend", "Frontend", "Own web and desktop product implementation", []string{"implementation", "ui"}),
		role("embedded", "Embedded", "Own device, robot and edge implementation", []string{"implementation", "edge"}),
		role("review", "Review", "Own code, architecture and security review", []string{"review", "security"}),
		role("test", "Test", "Own verification, regression and evidence", []string{"test", "evaluation"}),
		role("release", "Release", "Own build, release and deployment execution", []string{"release", "deployment"}),
	}
}

func role(key, name, description string, capabilities []string) OrganizationRole {
	return OrganizationRole{Key: key, Name: name, Description: description, Responsibilities: []string{}, RequiredSkills: []string{}, RecommendedSkills: []string{}, Capabilities: capabilities, Resolution: OrganizationResolution{State: "unresolved"}}
}

func FindOrganizationRole(key string) (OrganizationRole, error) {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, item := range OrganizationCatalog() { if item.Key == key { return item, nil } }
	return OrganizationRole{}, errors.New("unknown organization role")
}
