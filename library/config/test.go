package config

import gutils "github.com/Laisky/go-utils/v2"

func LoadTest() {
	if err := gutils.Settings.LoadFromFile("/opt/configs/go-ramjet/settings.yml"); err != nil {
		panic(err)
	}
}
