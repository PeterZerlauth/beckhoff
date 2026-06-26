package router

import (
	"encoding/json"
	"os"
)

// Coinfiguration 
type Config struct {
	AmsRouter struct {
		Name              string
		NetId             string
		RemoteConnections []RemoteConnection
	}
}

type RemoteConnection struct {
	Name    string
	Address string
	NetId   string
	Type    string
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	err = json.Unmarshal(data, &cfg)
	return &cfg, err
}
