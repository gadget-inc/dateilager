package main

import (
	"context"
	"os"
	"time"

	"github.com/angelini/dateilager/pkg/client"

	"go.uber.org/zap"
)

func main() {
	log, _ := zap.NewDevelopment()
	defer log.Sync()

	server := os.Getenv("SERVER")
	project := int32(1)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	c, err := client.NewClient(ctx, server)
	if err != nil {
		log.Fatal("could not connect to server", zap.String("server", server))
	}
	defer c.Close()

	objects, err := c.GetLatestRoot(ctx, project)
	if err != nil {
		log.Fatal("could not fetch data", zap.Error(err))
	}

	log.Info("listing objects in project", zap.Int32("project", project))
	for _, object := range objects {
		log.Info("object", zap.String("path", object.Path))
	}
}
