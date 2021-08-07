package main

import (
	"fmt"
	"gopkg.in/yaml.v2"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"
)

type Settings struct {
	RedisHost        string `yaml:"redis_host"`
	RedisPort        int    `yaml:"redis_port"`
	RuncWatchdogHost string `yaml:"runc_watchdog_host"`
	ReservedPorts    []int  `yaml:"reserved_ports"`
	CodeDir          string `yaml:"code_dir"`
	CheckpointDir    string `yaml:"checkpoint_dir"`
	PreStartTimeout  int    `yaml:"pre_start_timeout"`
	IdleTimeout      int    `yaml:"idle_timeout"`
	PreStartPoolSize int    `yaml:"pre_start_pool_size"`
	MaxPoolSize      int    `yaml:"max_pool_size"`
	MaxConcurrency   int    `yaml:"max_concurrency"`
}

type HealthCheck struct {
	Path   string `yaml:"path"`
	Wanted string `yaml:"wanted"`
}

type ServiceConfig struct {
	ServiceName       string      `yaml:"service_name"`
	ImageName         string      `yaml:"image_name"`
	TagName           string      `yaml:"tag_name"`
	CheckpointTagName string      `yaml:"checkpoint_tag_name"`
	CheckpointName    string      `yaml:"checkpoint_name"`
	StartCmd          []string    `yaml:"start_cmd"`
	ServicePort       int         `yaml:"service_port"`
	HostPort          int         `yaml:"host_port"`
	HealthCheck       HealthCheck `yaml:"health_check"`
	MsgCheckpoint     string      `yaml:"msg_checkpoint"`
	RwDirs            []string    `yaml:"rw_dirs"`
}

type void struct{}

var none void

func Exists(path string) bool {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsExist(err) {
			return true
		}
		return false
	}
	return true
}

func handleErr(err error) bool {
	if err != nil {
		if err != io.EOF {
			return true
		} else {
			return true
		}
	} else {
		return false
	}
}

func handleConn(conn *net.TCPConn, instance *ContainerInstance) {
	instance.LastVisitTime = time.Now()
	instance.ConnCount <- true
	defer func() {
		<-instance.ConnCount
	}()

	address := fmt.Sprintf("0.0.0.0:%v", instance.Port)
	serviceConn, err := net.Dial("tcp", address)

	defer conn.Close()
	defer serviceConn.Close()

	if err != nil {
		log.Printf("Fail to connect to %v: %v", address, err)
		return
	}

	exit := make(chan bool)

	go func() {
		var buff [512]byte
		for {
			m, err := conn.Read(buff[:])
			if handleErr(err) {
				exit <- true
				return
			}

			n := 0
			for i := 0; i < m; i += n {
				n, err = serviceConn.Write(buff[i:m])
				if handleErr(err) {
					exit <- true
					return
				}
			}
		}
	}()

	go func() {
		var buff [512]byte
		for {
			m, err := serviceConn.Read(buff[:])
			if handleErr(err) {
				exit <- true
				return
			}

			n := 0
			for i := 0; i < m; i += n {
				n, err = conn.Write(buff[i:m])
				if handleErr(err) {
					exit <- true
					return
				}
			}
		}
	}()

	select {
	case <-exit:
		return
	}
}

func getSettings() *Settings {
	yamlFile, err := ioutil.ReadFile("/etc/docker/settings.yaml")
	if err != nil {
		log.Fatalf("Get settings.yaml error: %v", err)
		return nil
	}
	var settings *Settings
	err = yaml.Unmarshal(yamlFile, &settings)
	if err != nil {
		log.Fatalf("Unmarshal settings.yaml error: %v", err)
		return nil
	}
	return settings
}

func getContainerConfig(pathConfig string) ServiceConfig {
	pathYamlFile := path.Join(pathConfig, "config.yaml")
	yamlFile, err := ioutil.ReadFile(pathYamlFile)
	if err != nil {
		log.Fatalf("Get config.yaml error: %v", err)
	}
	var config *ServiceConfig
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		log.Fatalf("Unmarshal config.yaml error: %v", err)
	}
	return *config
}

func getUsedPorts() map[int]void {
	set := make(map[int]void)

	res, err := exec.Command("bash", "-c", `netstat -ntl |grep -v Active| grep -v Proto|awk '{print $4}'|awk -F: '{print $NF}'| grep "[0-9]\{1,5\}"`).Output()
	if err != nil {
		log.Fatalf("Get used ports error: %v", err)
	}

	portsResStr := strings.Trim(string(res), "\n")
	portsStr := strings.Split(portsResStr, "\n")
	for _, port := range portsStr {
		portInt, err := strconv.Atoi(port)
		if err != nil {
			log.Fatalf("Atoi error: %v", err)
		}
		set[portInt] = none
	}

	return set
}

func healthCheck(port int, check HealthCheck) bool {
	client := &http.Client{
		Timeout: time.Millisecond * 100,
	}
	url := fmt.Sprintf("http://0.0.0.0:%v%v", port, check.Path)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false
	}
	res, err := client.Do(req)
	if err != nil {
		return false
	}
	var buff [4096]byte
	cnt, err := res.Body.Read(buff[:])
	if err != nil && err != io.EOF {
		return false
	}
	buffStr := string(buff[:cnt])
	if !strings.Contains(buffStr, check.Wanted) {
		return false
	}
	return true
}

func getInstancesInfo(instances []*ContainerInstance) map[ContainerStatus]int {
	res := map[ContainerStatus]int{
		Created: 0,
		Booting: 0,
		Running: 0,
		Stopped: 0,
		Full:    0,
	}
	for _, instance := range instances {
		res[instance.Status] += 1
		if instance.Status == Running && len(instance.ConnCount) >= settings.MaxConcurrency {
			res[Full] += 1
		}
	}
	return res
}
