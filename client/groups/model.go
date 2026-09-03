package groups

// Subject source identifiers, matching the values the groups service sets on
// Subject.SourceID.
const (
	SourceUser  = "ldap"
	SourceGroup = "g:gsa"
)

// GroupTypeSystem is the group type the DE's own internal groups carry, as
// opposed to the collaborator lists, teams, and communities users create.
const GroupTypeSystem = "system"

// Subject is one member of a group: an LDAP user or, when SourceID is
// "g:gsa", a nested group whose ID is the nested group's own group ID.
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

// Group is the groups service's representation of a single group.
type Group struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	GroupType   string `json:"group_type"`
	Owner       string `json:"owner"`
	Description string `json:"description"`
}

// GroupList is one page of a group listing response.
type GroupList struct {
	Groups []Group `json:"groups"`
}

// GroupMembers is a group's member listing response.
type GroupMembers struct {
	Members []Subject `json:"members"`

	// Redacted reports that the groups service withheld the member list
	// because the group is public but its membership is not. The list is
	// empty in that case but the group is not, so propagating it would strip
	// every member from the iRODS group.
	Redacted bool `json:"redacted"`

	// Total is the group's whole direct membership, which exceeds Members when
	// the response carries one page of it.
	Total int `json:"total"`
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
