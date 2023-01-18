package groups

type Subject struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	Email       string `json:"email"`
	Institution string `json:"institution"`
	Description string `json:"description"`

	AttributeValues []string `json:"attribute_values"`
}

type Group struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	DisplayName      string `json:"display_name"`
	Type             string `json:"type"`
	Description      string `json:"description"`
	Extension        string `json:"extension"`
	DisplayExtension string `json:"display_extension"`
	IDIndex          string `json:"id_index"`
} // should we add the 'detail' here?

type GroupsList struct {
	Groups []Group `json:"groups"`
}

type GroupMembers struct {
	Members []Subject `json:"members"`
}