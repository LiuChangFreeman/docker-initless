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

//Start sample services with CRIU checkpoint
systemctl start docker-initless

//Now you can visit the sample services on port 8001 and 8002
time curl http://0.0.0.0:8002
time curl http://0.0.0.0:8001?key=888888

//Look up status of docker-initless
systemctl status docker-initless
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