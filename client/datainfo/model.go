package datainfo

type User struct {
	Username string `json:"username"`
	Zone     string `json:"zone"`
}

type Group struct {
	Name    string   `json:"name"`
	Members []string `json:"members"`
}

type ParsedGroup struct {
	Name    string `json:"name"`
	Members []User `json:"members"`
}
