package main

import (
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"log"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path"
	"strconv"
	"time"
)

type ContainerStatus int

const (
	Created ContainerStatus = 0
	Booting ContainerStatus = 1
	Running ContainerStatus = 2
	Stopped ContainerStatus = 3
	Full    ContainerStatus = 4
)

type ContainerInstance struct {
	Status        ContainerStatus
	Id            string
	Name          string
	Port          int
	Config        ServiceConfig
	ConnCount     chan bool
	CreatTime     time.Time
	LastVisitTime time.Time
	BootTime      time.Time
}

func setCheckpointImages(instance *ContainerInstance) string {
	config := instance.Config
	containerId := instance.Port
	pathCheckpoint := settings.CheckpointDir
	checkpointName := config.CheckpointName

	pathCheckpointTemp := path.Join(pathCheckpoint, "temp")
	pathCheckpointUpper := path.Join(pathCheckpointTemp, fmt.Sprintf("v%v", containerId))
	pathCheckpointMerge := path.Join(pathCheckpointTemp, fmt.Sprintf("v%v-merge", containerId))
	pathCheckpointLower := path.Join(pathCheckpoint, checkpointName)

	if !Exists(pathCheckpointUpper) {
		err := os.Mkdir(pathCheckpointUpper, 777)
		if err != nil {
			log.Fatalf("Checkpoint imgs mkdir error: %v", err)
		}
	}

	if !Exists(pathCheckpointMerge) {
		err := os.Mkdir(pathCheckpointMerge, 777)
		if err != nil {
			log.Fatalf("Checkpoint imgs mkdir error: %v", err)
		}
	}

	cmdMount := fmt.Sprintf("mount -t overlay -o lowerdir=%v,upperdir=%v,workdir=%v overlay %v", pathCheckpointLower, pathCheckpointUpper, pathCheckpointMerge, pathCheckpointMerge)
	_, err := exec.Command("bash", "-c", cmdMount).Output()
	if err != nil {
		log.Fatalf("Mount overlay-fs error: %v", err)
	}
	return pathCheckpointMerge
}

func removeCheckpointImages(instance *ContainerInstance) {
	containerId := instance.Port
	pathCheckpoint := settings.CheckpointDir
	pathCheckpointTemp := path.Join(pathCheckpoint, "temp")
	pathCheckpointUpper := path.Join(pathCheckpointTemp, fmt.Sprintf("v%v", containerId))
	pathCheckpointMerge := path.Join(pathCheckpointTemp, fmt.Sprintf("v%v-merge", containerId))

	cmdUmount := fmt.Sprintf("umount -lf %v", pathCheckpointMerge)
	_, err := exec.Command("bash", "-c", cmdUmount).Output()
	if err != nil {
		log.Printf("Umount overlay-fs error: %v", err)
	}

	err = os.RemoveAll(pathCheckpointMerge)
	if err != nil {
		log.Printf("Remove checkpoint imgs error: %v", err)
	}
	err = os.RemoveAll(pathCheckpointUpper)
	if err != nil {
		log.Printf("Remove checkpoint imgs error: %v", err)
	}
}

func startLazyPageServer(pathCheckpoint string) {
	cmdStartLazyPageServer := fmt.Sprintf("criu lazy-pages --images-dir %v", pathCheckpoint)
	_, err := exec.Command("bash", "-c", cmdStartLazyPageServer).Output()
	if err != nil {
		log.Fatalf("Start lazy page server error: %v", err)
	}
}

func preStartContainer(instance *ContainerInstance) {
	containerId := instance.Port
	pathCheckpoint := settings.CheckpointDir
	pathCheckpointTemp := path.Join(pathCheckpoint, "temp")
	cmdPreStartContainer := fmt.Sprintf("docker start --checkpoint-dir=%v --checkpoint=v%v-merge %v", pathCheckpointTemp, containerId, instance.Name)
	_, err := exec.Command("bash", "-c", cmdPreStartContainer).Output()
	if err != nil {
		log.Printf("Pre-start container error: %v", err)
	}
	log.Printf("Container pre-start time: %v ms\n", time.Since(instance.BootTime).Milliseconds())
}

func removeContainer(instance *ContainerInstance) {
	err := dockerClient.ContainerRemove(ctx, instance.Id, types.ContainerRemoveOptions{Force: true})
	if err != nil {
		log.Printf("Remove container error: %v", err)
	}
	redisClient.Del(ctx, instance.Id)
	delete(allocatedPorts, instance.Port)
	log.Printf("Remove container: %v\n", instance.Name)
}

func startContainer(instance *ContainerInstance) {
	log.Printf("Start container %v\n", instance.Name)
	portWatchdog := instance.Port + 19000
	instance.BootTime = time.Now()
	_, _ = net.Dial("tcp", fmt.Sprintf("0.0.0.0:%v", portWatchdog))
	instance.Status = Running
}

func newContainerInstance(config ServiceConfig) *ContainerInstance {
	var portChosen int
	reservedPorts := settings.ReservedPorts
	usedPorts := getUsedPorts()
	for {
		port := reservedPorts[0] + rand.Intn(reservedPorts[1]-reservedPorts[0])
		_, exist := allocatedPorts[port]
		_, exist2 := usedPorts[port]
		if !exist && !exist2 {
			allocatedPorts[port] = none
			portChosen = port
			break
		}
	}

	containerName := fmt.Sprintf("%v-%v", config.ImageName, portChosen)
	log.Printf("Choose port %v\n", portChosen)

	instance := &ContainerInstance{
		Status:        Created,
		Name:          containerName,
		Port:          portChosen,
		Config:        config,
		ConnCount:     make(chan bool, settings.MaxConcurrency),
		CreatTime:     time.Now(),
		LastVisitTime: time.Now(),
		BootTime:      time.Now(),
	}

	servicePort := nat.Port(strconv.Itoa(config.ServicePort))

	dockerConfig := &container.Config{
		User: "root",
		Cmd:  config.StartCmd,
		ExposedPorts: map[nat.Port]struct{}{
			servicePort: {},
		},
		Image: fmt.Sprintf("%v:%v", config.ImageName, config.CheckpointTagName),
	}

	hostConfig := &container.HostConfig{
		SecurityOpt: []string{"seccomp=unconfined"},
		PortBindings: nat.PortMap{
			servicePort: []nat.PortBinding{{
				HostIP:   "0.0.0.0",
				HostPort: strconv.Itoa(portChosen),
			}},
		},
	}

	log.Println("Set checkpoint imgs using overlay-fs")
	pathCheckpoint := setCheckpointImages(instance)

	log.Println("Create container")
	containerCurrent, err := dockerClient.ContainerCreate(ctx, dockerConfig, hostConfig, nil, nil, containerName)
	if err != nil {
		log.Printf("Create container error: %v\n", err)
		return nil
	}
	instance.Id = containerCurrent.ID

	log.Println("Start lazy-pages server")
	go startLazyPageServer(pathCheckpoint)

	log.Println("Set container port")
	portWatchdog := 19000 + portChosen
	redisClient.Set(ctx, containerCurrent.ID, portWatchdog, 0)

	log.Println("Pre-start container")
	go preStartContainer(instance)

	return instance
}

func chooseOneContainerInstance(conn *net.TCPConn, config ServiceConfig) {
	waittingList := waittingLists[config.HostPort]
	waittingList <- true

	for range time.Tick(time.Millisecond * 25) {
		allContainerInstances := serviceInstances[config.HostPort]
		for _, instance := range allContainerInstances {
			if instance.Status == Running && len(instance.ConnCount) < settings.MaxConcurrency {
				go handleConn(conn, instance)
				goto out
			}
		}
	}
out:
}

func cleanUpStoppedContainerInstances(config ServiceConfig) {
	for range time.Tick(time.Second * 3) {
		allContainerInstances := serviceInstances[config.HostPort]
		for i := 0; i < len(allContainerInstances); i++ {
			instance := allContainerInstances[i]
			if instance.Status == Stopped {
				removeContainer(instance)
				removeCheckpointImages(instance)
				allContainerInstances = append(allContainerInstances[:i], allContainerInstances[i+1:]...)
				i--
			}
		}
	}
}

func scheduleContainerInstances(config ServiceConfig) {
	for range time.Tick(time.Millisecond * 500) {
		allContainerInstances := serviceInstances[config.HostPort]

		for _, instance := range allContainerInstances {
			if instance.Status == Running && time.Since(instance.LastVisitTime) >= time.Minute*time.Duration(settings.IdleTimeout) {
				instance.Status = Stopped
			}
			if instance.Status == Created && time.Since(instance.LastVisitTime) >= time.Minute*time.Duration(settings.PreStartTimeout) {
				instance.Status = Stopped
			}
		}

		containerInfo := getInstancesInfo(allContainerInstances)
		if containerInfo[Created] < settings.PreStartPoolSize {
			log.Printf("Spawn pre-start container pool")
			instance := newContainerInstance(config)
			if instance != nil {
				allContainerInstances = append(allContainerInstances, instance)
				serviceInstances[config.HostPort] = allContainerInstances
			}
		}
	}
}

func startContainerWatchdog(config ServiceConfig) {
	waittingList := waittingLists[config.HostPort]

	for {
		<-waittingList
		allContainerInstances := serviceInstances[config.HostPort]
		containerInfo := getInstancesInfo(allContainerInstances)
		if containerInfo[Running] == 0 {
			log.Printf("No running container")
			for _, instance := range allContainerInstances {
				if instance.Status == Created {
					startContainer(instance)
					break
				}
			}
		} else {
			if containerInfo[Full] >= containerInfo[Running] {
				log.Printf("All containers are full")
				if containerInfo[Running] < settings.MaxPoolSize {
					for _, instance := range allContainerInstances {
						if instance.Status == Created {
							startContainer(instance)
							break
						}
					}
				} else {
					log.Printf("Max pool size reached")
				}
			}
		}
	}
}

func spawnService(config ServiceConfig) {
	hostPort := config.HostPort
	_, exist := serviceInstances[hostPort]
	if exist {
		log.Fatalf("Dump service instance error: host port %v has been used", hostPort)
	}

	serviceInstances[hostPort] = []*ContainerInstance{}
	waittingLists[hostPort] = make(chan bool)

	address := net.TCPAddr{
		IP:   net.ParseIP("0.0.0.0"),
		Port: hostPort,
	}

	listener, err := net.ListenTCP("tcp", &address)
	if err != nil {
		log.Fatal("Fail to listen to: ", address, err)
	}

	go cleanUpStoppedContainerInstances(config)
	go scheduleContainerInstances(config)
	go startContainerWatchdog(config)

	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			log.Fatal("Fail to accept: ", err)
		}
		go chooseOneContainerInstance(conn, config)
	}
}

func testInitless(config ServiceConfig) {
	instance := newContainerInstance(config)

	log.Println("Start container after 3s")
	time.Sleep(time.Second * 3)

	log.Println("Try to start container")
	portWatchdog := instance.Port + 19000
	instance.BootTime = time.Now()
	_, _ = net.Dial("tcp", fmt.Sprintf("0.0.0.0:%v", portWatchdog))

	check := config.HealthCheck
	for {
		if time.Since(instance.BootTime) > time.Second*10 {
			log.Println("Start container failed: max tries reached")
			break
		}
		if healthCheck(instance.Port, check) {
			log.Printf("Total time: %v ms\n", time.Since(instance.BootTime).Milliseconds())
			break
		}
	}

	defer removeCheckpointImages(instance)
	defer removeContainer(instance)
}
