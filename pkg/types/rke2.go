package types

type RKE2Config struct {
	Token  string   `yaml:"token"`
	Server string   `yaml:"server"`
	TLSSan []string `yaml:"tls-san"`
}
