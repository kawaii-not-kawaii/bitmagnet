package auth

// Config controls human sessions, machine API keys, and trusted-network access.
type Config struct {
	Disabled        bool
	APIKey          string
	Username        string
	PasswordHash    string
	TrustedNetworks []string `validate:"dive,cidr"`
	TrustedProxies  []string `validate:"dive,cidr"`
}

func NewDefaultConfig() Config {
	return Config{}
}
