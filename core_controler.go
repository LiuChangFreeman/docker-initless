package main

import (
	"context"
	"fmt"
	"github.com/docker/docker/client"
	"github.com/go-redis/redis/v8"
	"io/ioutil"
	"log"
	"math/rand"
	"path"
	"time"
)

var (
	ctx              = context.Background()
	settings         = getSettings()
	redisClient      *redis.Client
	dockerClient     *client.Client
	allocatedPorts   = make(map[int]void)
	serviceInstances = make(map[int][]*ContainerInstance)
	waittingLists    = make(map[int]chan bool)
)

func main() {
	rand.Seed(time.Now().UTC().UnixNano())

	redisClient = redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%v:%v", settings.RedisHost, settings.RedisPort),
	})
	_, err := redisClient.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("Ping redis server error: %v", err)
	}

	dockerClient, err = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("New docker client error: %v", err)
	}

	codeDir := settings.CodeDir
	pathCodeDirs, err := ioutil.ReadDir(codeDir)
	if err != nil {
		log.Fatalf("List code dir error: %v", err)
	}

	for _, pathCodeDir := range pathCodeDirs {
		pathCurrent := path.Join(codeDir, pathCodeDir.Name())
		containerConfig := getContainerConfig(pathCurrent)
		go spawnService(containerConfig)
	}

	select {}
}
