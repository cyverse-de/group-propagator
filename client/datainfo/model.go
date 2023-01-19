package datainfo

type User struct {
	Username string `json:"username"`
	Zone     string `json:"zone"`
}

type Group struct {
	Name    string   `json:"name,omitempty"`
	Members []string `json:"members"`
}
