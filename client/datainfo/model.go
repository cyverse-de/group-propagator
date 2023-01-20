package datainfo

type User struct {
	Username string `json:"username"`
	Zone     string `json:"zone"`
}

type Group struct {
	Name    string   `json:"name,omitempty"`
	Members []string `json:"members"`
}

type ServiceError struct {
	ErrorCode string `json:"error_code"`
}
