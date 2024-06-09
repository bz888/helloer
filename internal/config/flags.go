package config

import "flag"

var (
	ConfigPath string
	SavePath   string
)

func InitFlags() {
	flag.StringVar(&ConfigPath, "configPath", "", "path to config file")
	flag.StringVar(&SavePath, "savePath", "", "path to save file")
	flag.Parse()
}
