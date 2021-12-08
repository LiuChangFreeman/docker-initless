# Docker-initless
Boot containers using `checkpoint && restore`    
Eliminate [gs-spring-boot](https://github.com/LiuChangFreeman/gs-spring-boot) docker container's cold start time from **2500+ms** to **400-ms**   
Cost saving along with better performance as on-remand Docker service.  
## Dependencies  

1. Docker CE==17.03
2. criu from  https://github.com/LiuChangFreeman/criu
3. runc from  https://github.com/LiuChangFreeman/runc
4. containerd from  https://github.com/LiuChangFreeman/containerd 

## Requirements

Currently `docker-initless` is developed on a normal server with regular hardware _(2 vCore-4 GB, Centos 7 , without fast SSD)_ . All you need is to build the docker-in-docker image and give the **--privileged** flag to your `docker-initless` container instance. 

## How does it work?
The simplest `hello-world` container will still costs about 700ms to boot(Many people hate cold start of FaaS, which comes from this):
```c++
root@VM-4-7-ubuntu:~# time docker run --rm hello-world

Hello from Docker!
This message shows that your installation appears to be working correctly.

To generate this message, Docker took the following steps:
 1. The Docker client contacted the Docker daemon.
 2. The Docker daemon pulled the "hello-world" image from the Docker Hub.
    (amd64)
 3. The Docker daemon created a new container from that image which runs the
    executable that produces the output you are currently reading.
 4. The Docker daemon streamed that output to the Docker client, which sent it
    to your terminal.

To try something more ambitious, you can run an Ubuntu container with:
 $ docker run -it ubuntu bash

Share images, automate workflows, and more with a free Docker ID:
 https://hub.docker.com/

For more examples and ideas, visit:
 https://docs.docker.com/get-started/


real    0m0.682s
user    0m0.019s
sys     0m0.012s
```  
`docker-initless` works with the **criu** project, which is known as a tool which can recover a group of froozen processes on another machine. This tool can be used to perform `AOT` optimization of Docker containers cold start.   
`docker-initless` uses the `post-copy restore` to make a container runable before the pages are filled into memory which is a background task. Time spent on restoring memory pages at *GB* level will be reduced significantly.     
The other `pre-start` mechanism can also save about 300+ms(Most of it comes from creating `network namespace`). `docker-initless` creates and starts a container but doesn't allow CRIU to restore memory pages until it connects the port that runc watches. This means that `network namespace` can be ready ahead of time before CPU and memory are allocated when there are requests to handle. So the cost of `pre-start` container pool is very low.

## Which kind of service can be optimized with docker-initless?
Current limitation:  
1. Can only be HTTP service
2. Must be state-less. Instances can be removed immediately after requests are handled so don't use background tasks. 
3. Must handle errors when connections are broken. This means that if your service uses redis/mysql/mongodb, you should check whether the connections were dead and do reconnect.
4. Must print a message to stdout when the service is ready for connections(See `msg_checkpoint` defined in `config.yaml`). `docker-initless` will try to get logs of the container once every 3s and check whether the message were printed. If so, it will let CRIU to checkpont the container.
5. No support for mount volumes(TODO).

## Let's setup  
### 1. Build the docker-in-docker image
```js
//We suppose you have installed Docker on your machine
git clone https://github.com/LiuChangFreeman/docker-initless.git
cd docker-initless

//Build the image using Dockerfile 
docker build -t docker-initless:dind .

//Prepare a outside-folder to store the containers and images used by docker-initless
mkdir -p /var/lib/docker-initless

//Create a docker-in-docker container instance using the outside Docker
docker run --name docker-initless -d --privileged --net=host --restart=always -v $(pwd):/home -v /var/lib/docker-initless:/var/lib/docker -v /tmp:/tmp -v /lib/modules/:/lib/modules/ docker-initless:dind /usr/sbin/init

//Now we enter the container
docker exec -it  docker-initless /bin/bash  
```
### 2. Generate asserts of the service
Note: **all these operations should be done INSIDE the container created above** 
```bash
//Setup a redis server using the inside Docker(redis_host is set to 0.0.0.0 by default so we need to run it inside)
docker run -p 0.0.0.0:6380:6379 --name redis --restart=always -d redis redis-server

//Create sample hello-world service images and CRIU checkpoints using the python script
python checkpoint_manager.py
//The CRIU checkpoints will be located in /home/checkpoint by default if success

//Run tests with the CRIU checkpoints comparing with normal docker-run cases
python run_tests.py

//Start sample services with CRIU checkpoint
systemctl start docker-initless

//Now you can visit the sample services on port 8001 and 8002
time curl http://0.0.0.0:8002
time curl http://0.0.0.0:8001?key=888888

//Look up status of docker-initless
systemctl status docker-initless
```
## Configuration
The global configuration file: `/etc/docker/settings.yaml`
```yaml
redis_port: #The port of redis used by docker-initless
    6380 
redis_host: #The ip address of redis used by docker-initless
    "0.0.0.0"
runc_watchdog_host: #The ip address to be listened by runc when performing pre-start 
    "0.0.0.0"
runc_watchdog_port_base: #OFFSET added to the port used by container inner service. Set it to the length of `reserved_ports` 
    10000
reserved_ports: #The ports used by container inner service. For mutiple container instances, everyone will take up a port individally.
  [10000,20000]
code_dir: #The code folder of mutiple Docker services. Each should contain a dockerfile and a config.yaml
    "/home/sample"
checkpoint_dir: #The folder used to store CRIU checkpoints. `temp` will be created automaticlly and used for creating temp containers.
    "/home/checkpoint"
recycle_workers: #Jobs to remove `dead` containers. 
    4
pre_start_pool_size: #Number of the containers which are pre-started and can be booted at once.
    2
max_pool_size: #Max size of containers which are running and can handle requests.
    4
pre_start_timeout: #Must be lower than 24*60. The minutes that a pre-start container can keep alive.
    15
idle_timeout: #The minutes that a running container can keep alive. When there aren't any requests to handle, running containers will be stoped and removed.
    1
health_check_timeout: #The seconds that a booting container can take most when being health checking. After which the booting container will be reguarded as a failure.
    10
max_concurrency: #The number of connections that a running container can handle mostly. If beyond docker-initless will start a new one to handle the other.
    100
```
Each configuration file located in the sub dirs of `code_dir` : `config.yaml` 
```yaml
service_name: #The name of this service.
  gs_spring_boot
is_enabled: #Is it ready to be published?
  true
image_name: #The name that will be used to the image that built with local dockerfile.
  gs_spring_boot
tag_name: #The tag name used when building.
  init
checkpoint_tag_name: #The tag name used when checkpointing.
  latest
checkpoint_name: #The folder name used to store `.img`s when checkpointing.
  spring_boot_checkpoint
start_cmd: #How to start the inner service directly when creating a Docker comntainer?
  - java
  - -jar
  - spring-boot-complete-0.0.1-SNAPSHOT.jar
service_port: #The port that will be used by the inner service inside the container.
  8080
host_port: #The port that you want to publish the service outside the container.
  8002
health_check: #Http GET sent to a running container to test if it has been ready.
  path: "/" #The http path and query to be tested.
  wanted: "Greetings from Spring Boot!" #Words that success response should contain.  
  timeout: 150 #How many milliseconds to be waited once?
msg_checkpoint: #Words that the inner service will print when it is ready.
  "Started Application"
```
## Examples
Use `docker-initless -t` or `python run_tests.py` to make tests with the CRIU checkpoints.   
Use `docker-initless -clean` or `sh clean.sh` to make a totally clean-up when you get errors.  
```bash
[root@iZuf66kclfla82jiha2r23Z home]# docker-initless -t
2021/12/07 05:20:10 [flask] Create container flask-12643
2021/12/07 05:20:10 [flask-12643] Will start container after 3s
2021/12/07 05:20:13 [flask-12643] Try to start container
2021/12/07 05:20:13 [flask-12643] Health check latency test result is: 81 ms
2021/12/07 05:20:13 [flask-12643] Start to remove
2021/12/07 05:20:13 [flask-12643] Remove finished
2021/12/07 05:20:13 [gs_spring_boot] Create container gs_spring_boot-17589
2021/12/07 05:20:13 [gs_spring_boot-17589] Will start container after 3s
2021/12/07 05:20:16 [gs_spring_boot-17589] Try to start container
2021/12/07 05:20:17 [gs_spring_boot-17589] Health check latency test result is: 386 ms
2021/12/07 05:20:17 [gs_spring_boot-17589] Start to remove
2021/12/07 05:20:17 [gs_spring_boot-17589] Remove finished
```
```
[root@iZuf66kclfla82jiha2r23Z home]# time curl http://0.0.0.0:8002
Greetings from Spring Boot!
real    0m0.399s
user    0m0.001s
sys     0m0.003s
[root@iZuf66kclfla82jiha2r23Z home]# time curl http://0.0.0.0:8001?key=888888
{"value": 888888}
real    0m0.085s
user    0m0.001s
sys     0m0.003s
```