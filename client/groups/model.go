package groups

type Subject struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	Email       string `json:"email"`
	Institution string `json:"institution"`
	Description string `json:"description"`
	SourceID    string `json:"source_id"`

	AttributeValues []string `json:"attribute_values"`
}

type Group struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	GroupType   string `json:"group_type"`
	Owner       string `json:"owner"`
	Description string `json:"description"`
}

type GroupList struct {
	Groups []Group `json:"groups"`
}

type GroupMembers struct {
	Members []Subject `json:"members"`

	// Redacted reports that the groups service withheld the member list
	// because the group is public but its membership is not. The list is
	// empty in that case but the group is not, so propagating it would strip
	// every member from the iRODS group.
	Redacted bool `json:"redacted"`
}

// Status is the groups service's GET / response. Keycloak is deliberately not
// treated as fatal: without it names and email addresses degrade, but group
// membership -- the only thing this service reads -- still resolves.
type Status struct {
	Service  string `json:"service"`
	Version  string `json:"version"`
	Database bool   `json:"database"`
	Keycloak bool   `json:"keycloak"`
}
