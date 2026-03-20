package sources

// Credentials holds authentication information for remote providers.
type Credentials struct {
	Token    string `json:"token"`
	Username string `json:"username"`
	Password string `json:"password"`
}
