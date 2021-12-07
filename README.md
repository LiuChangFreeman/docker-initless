# Docker-initless
Boot containers using `checkpoint && restore`    
Eliminate [gs-spring-boot](https://github.com/LiuChangFreeman/gs-spring-boot) docker container's cold start time from **2500+ ms** to **300+ ms**   
Cost saving along with better performance as on-remand Docker service.  
## Dependencies  

1. Docker CE==17.03
2. criu from  https://github.com/LiuChangFreeman/criu
3. runc from  https://github.com/LiuChangFreeman/runc
4. containerd from  https://github.com/LiuChangFreeman/containerd 

## Requirements

Currently `docker-initless` is developed on a normal server with regular hardware _(2 vCore-4 GB, Centos 7 , without fast SSD)_ . All you need is to build the docker-in-docker image and give the **--privileged** flag to your `docker-initless` container instance. 

## How does it work?

`docker-initless` works with the **criu** project, which is known as a tool which can recover a group of froozen processes on another machine.   
`docker-initless` uses the `post-copy restore` to make a container runable before the pages are filled into memory which is a background task. Time spent on restoring memory pages at *GB* level will be reduced significantly.     
The other `pre-start mechanism` can also reduce several hundreds of millis during the total container boot precedure.  


## Let's setup  
```c++
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

//Setup a redis server using the inside Docker(If you do it outside there may be some bugs)
docker run -p 0.0.0.0:6380:6379 --name redis --restart=always -d redis redis-server

//Create sample hello-world service images and CRIU checkpoints using the python script
python checkpoint_manager.py
//The CRIU checkpoints will be located in /home/checkpoint by default if success

//Run tests with the CRIU checkpoints comparing with normal docker-run cases
python run_tests.py

```
## Samples
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

```bash
[root@iZuf66kclfla82jiha2r23Z home]# python run_tests.py 
12/07/21 05:22:10 [gs_spring_boot] Start to test initless boot
12/07/21 05:22:10 --------Epoch 1--------
12/07/21 05:22:10 [gs_spring_boot] Prepare checkpoint imgs using overlay-fs
12/07/21 05:22:10 [gs_spring_boot] Create container
12/07/21 05:22:10 [gs_spring_boot] Start lazy-pages server
12/07/21 05:22:10 [gs_spring_boot] Set watchdog port
12/07/21 05:22:10 [gs_spring_boot] Pre-start container
12/07/21 05:22:10 [gs_spring_boot] Will start the container after 3s
12/07/21 05:22:13 [gs_spring_boot] Try to start container
12/07/21 05:22:13 [gs_spring_boot] Container started
12/07/21 05:22:13 [gs_spring_boot] Docker start time: 0.072777s
12/07/21 05:22:13 [gs_spring_boot] Health check want: "Greetings from Spring Boot!"
12/07/21 05:22:13 [gs_spring_boot] Got: "Greetings from Spring Boot!", assert success
12/07/21 05:22:13 [gs_spring_boot] Request totally cost: 0.370128s
12/07/21 05:22:13 [gs_spring_boot] Test finished
12/07/21 05:22:13 --------Epoch end--------
12/07/21 05:22:13 [gs_spring_boot] Start to test normal boot
12/07/21 05:22:13 --------Epoch 1--------
12/07/21 05:22:13 [gs_spring_boot] Run container
12/07/21 05:22:16 [gs_spring_boot] Health check want: "Greetings from Spring Boot!"
12/07/21 05:22:16 [gs_spring_boot] Got: "Greetings from Spring Boot!", assert success
12/07/21 05:22:16 [gs_spring_boot] Total cost: 2.571254s
12/07/21 05:22:16 [gs_spring_boot] Test finished
12/07/21 05:22:16 --------Epoch end--------
12/07/21 05:22:16 [flask] Start to test initless boot
12/07/21 05:22:16 --------Epoch 1--------
12/07/21 05:22:16 [flask] Prepare checkpoint imgs using overlay-fs
12/07/21 05:22:16 [flask] Create container
12/07/21 05:22:16 [flask] Start lazy-pages server
12/07/21 05:22:16 [flask] Set watchdog port
12/07/21 05:22:16 [flask] Pre-start container
12/07/21 05:22:16 [flask] Will start the container after 3s
12/07/21 05:22:19 [flask] Try to start container
12/07/21 05:22:19 [flask] Container started
12/07/21 05:22:19 [flask] Docker start time: 0.131514s
12/07/21 05:22:19 [flask] Health check want: "9999"
12/07/21 05:22:19 [flask] Got: "{"value": 9999}", assert success
12/07/21 05:22:19 [flask] Request totally cost: 0.137314s
12/07/21 05:22:20 [flask] Test finished
12/07/21 05:22:20 --------Epoch end--------
12/07/21 05:22:20 [flask] Start to test normal boot
12/07/21 05:22:20 --------Epoch 1--------
12/07/21 05:22:20 [flask] Run container
12/07/21 05:22:24 [flask] Health check want: "9999"
12/07/21 05:22:24 [flask] Got: "{"value": 9999}", assert success
12/07/21 05:22:24 [flask] Total cost: 3.922676s
12/07/21 05:22:24 [flask] Test finished
12/07/21 05:22:24 --------Epoch end--------
```
