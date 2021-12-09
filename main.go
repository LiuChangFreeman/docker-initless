package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/docker/docker/client"
	"github.com/go-redis/redis/v8"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"path"
	"sync"
	"time"
)

var (
	showHelp    bool
	runTests    bool
	doServe     bool
	cleanUp     bool
	showVersion bool

	stopedInstances   = make(chan *ContainerInstance)
	boottingContainer = make(chan void, 1)
	ctx               = context.Background()
	settings          = getSettings()
	redisClient       *redis.Client
	dockerClient      *client.Client

	allocatedPorts = struct {
		sync.RWMutex
		data map[int]void
	}{data: make(map[int]void)}

	serviceInstances = struct {
		sync.RWMutex
		data map[int][]*ContainerInstance
	}{data: make(map[int][]*ContainerInstance)}

	waittingConnsChans = struct {
		sync.RWMutex
		data map[int]chan void
	}{data: make(map[int]chan void)}

	readyContainersLists = struct {
		sync.RWMutex
		data map[int]chan *ContainerInstance
	}{data: make(map[int]chan *ContainerInstance)}
)

func init() {

	flag.BoolVar(&showHelp, "h", false, "show help")
	flag.BoolVar(&runTests, "t", false, "run tests using initless mode")
	flag.BoolVar(&doServe, "d", false, "start daemon service")
	flag.BoolVar(&cleanUp, "clean", false, "clean up all containers, except for redis")
	flag.BoolVar(&showVersion, "v", false, "get version")
	flag.Parse()

	if !runTests && !doServe && !cleanUp {
		if showVersion {
			getVersion()
		} else {
			flag.Usage()
		}
		os.Exit(0)
	}

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

	if cleanUp {
		cleanUpContainers()
		os.Exit(0)
	}
}

func main() {
	codeDir := settings.CodeDir
	pathCodeDirs, err := ioutil.ReadDir(codeDir)
	if err != nil {
		log.Fatalf("List code dir error: %v", err)
	}

	if runTests {
		for _, pathCodeDir := range pathCodeDirs {
			pathCurrent := path.Join(codeDir, pathCodeDir.Name())
			containerConfig := getContainerConfig(pathCurrent)
			if containerConfig.IsEnabled {
				testInitless(containerConfig)
			}
		}
	}

	if doServe {

		for i := 0; i < settings.RecycleWorkers; i++ {
			go cleanUpStoppedContainerInstances()
		}

		for _, pathCodeDir := range pathCodeDirs {
			pathCurrent := path.Join(codeDir, pathCodeDir.Name())
			containerConfig := getContainerConfig(pathCurrent)
			if containerConfig.IsEnabled {
				go initService(containerConfig)
			}
		}
		select {}
	}
}
