package config

import gconfig "github.com/Laisky/go-config"

func LoadTest() {
	if err := gconfig.Shared.LoadFromFile("/opt/configs/go-ramjet/settings.yml"); err != nil {
		panic(err)
	}
}
