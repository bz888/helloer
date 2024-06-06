package config

import "flag"

var (
	ConfigPath string
)

func InitFlags() {
	flag.StringVar(&ConfigPath, "file", "", "path to config file")
	flag.Parse()
}
