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

func setCheckpointImages(instance *ContainerInstance) {
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
			log.Printf("[%v] Checkpoint imgs mkdir error: %v\n", instance.Name, err)
		}
	}

	if !Exists(pathCheckpointMerge) {
		err := os.Mkdir(pathCheckpointMerge, 777)
		if err != nil {
			log.Printf("[%v] Checkpoint imgs mkdir error: %v", instance.Name, err)
		}
	}

	cmdMount := fmt.Sprintf("mount -t overlay -o lowerdir=%v,upperdir=%v,workdir=%v overlay %v", pathCheckpointLower, pathCheckpointUpper, pathCheckpointMerge, pathCheckpointMerge)
	_, err := exec.Command("bash", "-c", cmdMount).Output()
	if err != nil {
		log.Printf("[%v] Mount overlay-fs error: %v", instance.Name, err)
	}
}

func removeCheckpointImages(instance *ContainerInstance) {
	containerId := instance.Port
	pathCheckpoint := settings.CheckpointDir
	pathCheckpointTemp := path.Join(pathCheckpoint, "temp")
	pathCheckpointUpper := path.Join(pathCheckpointTemp, fmt.Sprintf("v%v", containerId))
	pathCheckpointMerge := path.Join(pathCheckpointTemp, fmt.Sprintf("v%v-merge", containerId))

	cmdUmount := fmt.Sprintf("umount -lf %v", pathCheckpointMerge)
	_, _ = exec.Command("bash", "-c", cmdUmount).Output()

	_ = os.RemoveAll(pathCheckpointMerge)

	_ = os.RemoveAll(pathCheckpointUpper)
}

func startLazyPageServer(instance *ContainerInstance) {
	containerId := instance.Port
	pathCheckpoint := settings.CheckpointDir
	pathCheckpointTemp := path.Join(pathCheckpoint, "temp")
	pathCheckpointMerge := path.Join(pathCheckpointTemp, fmt.Sprintf("v%v-merge", containerId))

	_, err := exec.Command("/usr/local/bin/criu", "lazy-pages", "--images-dir", pathCheckpointMerge).Output()
	if err != nil {
		if instance.Status != Stopped {
			log.Printf("[%v] Start lazy-pages error: %v", instance.Name, err)
		}
	}
}

func preStartContainer(instance *ContainerInstance) {
	containerId := instance.Port
	pathCheckpoint := settings.CheckpointDir
	pathCheckpointTemp := path.Join(pathCheckpoint, "temp")
	err := dockerClient.ContainerStart(ctx, instance.Id, types.ContainerStartOptions{CheckpointDir: pathCheckpointTemp, CheckpointID: fmt.Sprintf("v%v-merge", containerId)})
	if err != nil {
		if instance.Status != Stopped {
			log.Printf("[%v] Pre-start container error: %v\n", instance.Name, err)
		}
	}
}

func removeContainer(instance *ContainerInstance) {
	containerId := instance.Port
	_, _ = net.Dial("tcp", fmt.Sprintf("%v:%v", settings.RuncWatchdogHost, instance.Port+settings.RuncWatchdogPortBase))
	log.Printf("[%v] Start to remove\n", instance.Name)

	cmdKillPageServer := fmt.Sprintf("ps -ef| grep 'v%v-merge'  | awk '{print $2}' |xargs kill -9", containerId)
	_, _ = exec.Command("bash", "-c", cmdKillPageServer).Output()

	cmdKillDockerRunc := fmt.Sprintf("ps -ef| grep '%v'  | awk '{print $2}' |xargs kill -9", instance.Id)
	_, _ = exec.Command("bash", "-c", cmdKillDockerRunc).Output()

	_ = dockerClient.ContainerRemove(ctx, instance.Id, types.ContainerRemoveOptions{Force: true})
	redisClient.Del(ctx, instance.Id)
	allocatedPorts.Lock()
	delete(allocatedPorts.data, instance.Port)
	allocatedPorts.Unlock()
	log.Printf("[%v] Remove finished\n", instance.Name)
}

func startContainer(instance *ContainerInstance) {
	config := instance.Config
	waittingConnsChan := waittingConnsChans.data[config.HostPort]
	readyContainersList := readyContainersLists.data[config.HostPort]
	instance.BootTime = time.Now()
	_, _ = net.Dial("tcp", fmt.Sprintf("%v:%v", settings.RuncWatchdogHost, instance.Port+settings.RuncWatchdogPortBase))

	for {
		if time.Since(instance.BootTime) > time.Second*time.Duration(settings.HealthCheckTimeout) {
			log.Printf("[%v] Health check max tries reached\n", instance.Name)
			instance.Status = Stopped
			waittingConnsChan <- none
			break
		}
		if healthCheck(instance) {
			log.Printf("[%v] Health check latency test result is: %v ms\n", instance.Name, time.Since(instance.BootTime).Milliseconds())
			instance.Status = Running
			readyContainersList <- instance
			log.Printf("[%v] Instance ready\n", instance.Name)
			break
		}
	}

	<-boottingContainer
}

func newContainerInstance(config ServiceConfig) *ContainerInstance {
	var portChosen int
	reservedPorts := settings.ReservedPorts
	usedPorts := getUsedPorts()
	for {
		port := reservedPorts[0] + rand.Intn(reservedPorts[1]-reservedPorts[0])
		_, exist := allocatedPorts.data[port]
		_, exist2 := usedPorts[port]

		portWatchdog := port + settings.RuncWatchdogPortBase
		_, exist3 := allocatedPorts.data[portWatchdog]
		_, exist4 := usedPorts[portWatchdog]

		if !exist && !exist2 && !exist3 && !exist4 {
			allocatedPorts.Lock()
			allocatedPorts.data[port] = none
			allocatedPorts.Unlock()
			portChosen = port
			break
		}
	}

	containerName := fmt.Sprintf("%v-%v", config.ImageName, portChosen)

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
	setCheckpointImages(instance)

	log.Printf("[%v] Create container %v\n", config.ServiceName, containerName)
	containerCurrent, err := dockerClient.ContainerCreate(ctx, dockerConfig, hostConfig, nil, nil, containerName)
	if err != nil {
		log.Printf("[%v] Create container error: %v\n", config.ServiceName, err)
		return nil
	}
	instance.Id = containerCurrent.ID

	go startLazyPageServer(instance)

	portWatchdog := settings.RuncWatchdogPortBase + portChosen
	redisClient.Set(ctx, instance.Id, portWatchdog, 0)

	go preStartContainer(instance)

	return instance
}

func chooseOneContainerInstance(conn *net.TCPConn, config ServiceConfig) {
	waittingConnsChan := waittingConnsChans.data[config.HostPort]
	readyContainersList := readyContainersLists.data[config.HostPort]
	waittingConnsChan <- none
	select {
	case instance := <-readyContainersList:
		handleConn(conn, instance)
		break
	}
}

func cleanUpStoppedContainerInstances() {
	for {
		select {
		case instance := <-stopedInstances:
			done := make(chan void, 1)
			go func() {
				removeCheckpointImages(instance)
				removeContainer(instance)
				done <- none
			}()

			select {
			case <-done:
				break
			case <-time.After(time.Second * 30):
				log.Printf("[Warning] Remove container time out\n")
				break
			}
		}
	}
}

func scheduleContainerInstances(config ServiceConfig) {
	for range time.Tick(time.Millisecond * 100) {
		serviceInstances.Lock()
		allContainerInstances := serviceInstances.data[config.HostPort]
		for i := 0; i < len(allContainerInstances); i++ {
			instance := allContainerInstances[i]
			if (instance.Status == Running && time.Since(instance.LastVisitTime) >= time.Minute*time.Duration(settings.IdleTimeout)) ||
				(instance.Status == Created && time.Since(instance.LastVisitTime) >= time.Minute*time.Duration(settings.PreStartTimeout)) || instance.Status == Stopped {
				instance.Status = Stopped
				stopedInstances <- instance
				allContainerInstances = append(allContainerInstances[:i], allContainerInstances[i+1:]...)
				i--
			}
		}
		serviceInstances.data[config.HostPort] = allContainerInstances
		serviceInstances.Unlock()

		containerInfo := getInstancesInfo(allContainerInstances)
		if containerInfo[Created] < settings.PreStartPoolSize {
			boottingContainer <- none
			log.Printf("[%v] Spawn pre-start container\n", config.ServiceName)
			instance := newContainerInstance(config)
			<-boottingContainer
			if instance != nil {
				serviceInstances.Lock()
				serviceInstances.data[config.HostPort] = append(serviceInstances.data[config.HostPort], instance)
				serviceInstances.Unlock()
			}
		}
	}
}

func startContainerWatchdog(config ServiceConfig) {
	waittingList := waittingConnsChans.data[config.HostPort]
	readyContainersList := readyContainersLists.data[config.HostPort]

	for {
		<-waittingList
		serviceInstances.RLock()
		allContainerInstances := serviceInstances.data[config.HostPort]
		serviceInstances.RUnlock()
		containerInfo := getInstancesInfo(allContainerInstances)

		if containerInfo[Booting] > 0 {
			go func() {
				waittingList <- none
			}()
		} else {
			if containerInfo[Running] == 0 {
				if containerInfo[Created] > 0 {
					for _, instance := range allContainerInstances {
						if instance.Status == Created {
							log.Printf("[%v] No running container, start %v\n", config.ServiceName, instance.Name)
							boottingContainer <- none
							instance.Status = Booting
							go startContainer(instance)
							break
						}
					}
				} else {
					log.Printf("[%v] Pool is empty\n", config.ServiceName)
					time.Sleep(time.Millisecond * 50)
					waittingList <- none
				}
			} else {
				if containerInfo[Full] >= containerInfo[Running] {
					if containerInfo[Running] < settings.MaxPoolSize {
						for _, instance := range allContainerInstances {
							if instance.Status == Created {
								log.Printf("[%v]All containers are full, start %v\n", config.ServiceName, instance.Name)
								boottingContainer <- none
								instance.Status = Booting
								go startContainer(instance)
								break
							}
						}
					} else {
						log.Printf("[%v] Max pool size reached\n", config.ServiceName)
						time.Sleep(time.Millisecond * 50)
						waittingList <- none
					}
				} else {
					for _, instance := range allContainerInstances {
						if instance.Status == Running && len(instance.ConnCount) < settings.MaxConcurrency {
							readyContainersList <- instance
							break
						}
					}
				}
			}
		}
	}
}

func initService(config ServiceConfig) {
	hostPort := config.HostPort
	serviceInstances.RLock()
	_, exist := serviceInstances.data[hostPort]
	serviceInstances.RUnlock()
	if exist {
		log.Fatalf("[%v] Host port %v has been used", config.ServiceName, hostPort)
	}

	waittingConnsChans.Lock()
	waittingConnsChans.data[hostPort] = make(chan void)
	waittingConnsChans.Unlock()

	readyContainersLists.Lock()
	readyContainersLists.data[hostPort] = make(chan *ContainerInstance)
	readyContainersLists.Unlock()

	serviceInstances.Lock()
	serviceInstances.data[hostPort] = []*ContainerInstance{}
	serviceInstances.Unlock()

	address := net.TCPAddr{
		IP:   net.ParseIP("0.0.0.0"),
		Port: hostPort,
	}

	listener, err := net.ListenTCP("tcp", &address)
	if err != nil {
		log.Fatalf("[%v] Fail to listen to %v: %v\n", config.ServiceName, address, err)
	}

	go scheduleContainerInstances(config)
	go startContainerWatchdog(config)

	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			log.Fatalf("[%v] Fail to accept: %v\n", config.ServiceName, err)
		}
		go chooseOneContainerInstance(conn, config)
	}
}

func testInitless(config ServiceConfig) {
	instance := newContainerInstance(config)

	log.Printf("[%v] Will start container after 3s", instance.Name)
	time.Sleep(time.Second * 3)

	log.Printf("[%v] Try to start container", instance.Name)
	instance.BootTime = time.Now()
	_, _ = net.Dial("tcp", fmt.Sprintf("%v:%v", settings.RuncWatchdogHost, instance.Port+settings.RuncWatchdogPortBase))

	for {
		if time.Since(instance.BootTime) > time.Second*time.Duration(settings.HealthCheckTimeout) {
			log.Printf("[%v] Health check max tries reached\n", instance.Name)
			break
		}
		if healthCheck(instance) {
			log.Printf("[%v] Health check latency test result is: %v ms\n", instance.Name, time.Since(instance.BootTime).Milliseconds())
			break
		}
	}

	defer removeCheckpointImages(instance)
	defer removeContainer(instance)
}

func cleanUpContainers() {
	pathCheckpoint := settings.CheckpointDir
	pathCheckpointTemp := path.Join(pathCheckpoint, "temp")
	cmdUmount := fmt.Sprintf("umount -lf %v/*", pathCheckpointTemp)
	_, _ = exec.Command("bash", "-c", cmdUmount).Output()

	_ = os.RemoveAll(pathCheckpointTemp)
	_ = os.Mkdir(pathCheckpointTemp, 0777)

	cmdKillPageServers := "ps -ef| grep 'criu'  | awk '{print $2}' |xargs kill -9"
	_, _ = exec.Command("bash", "-c", cmdKillPageServers).Output()

	cmdKillDockerRunc := "ps -ef| grep 'docker-runc'  | awk '{print $2}' |xargs kill -9"
	_, _ = exec.Command("bash", "-c", cmdKillDockerRunc).Output()

	containers, _ := dockerClient.ContainerList(ctx, types.ContainerListOptions{All: true})
	for _, instance := range containers {
		if instance.Names[0][1:] == settings.RedisName {
			continue
		}
		_ = dockerClient.ContainerRemove(ctx, instance.ID, types.ContainerRemoveOptions{Force: true})
	}
}
