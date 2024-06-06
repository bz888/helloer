package cmd

import (
	"context"
	"fmt"
	"github.com/bz888/helloer/internal/config"
	"log"
)

func Execute() {

	config.InitFlags()
	modelConfig, err := config.LoadConfig(config.ConfigPath)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Starting CLI application. Type 'exit' to quit.")
	ctx, cancel := context.WithCancel(context.Background())
	Generate(ctx, modelConfig)
	defer cancel()
}
