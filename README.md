# Docker-initless
Boot containers using `checkpoint && restore`    
Eliminate [gs-spring-boot](https://github.com/LiuChangFreeman/gs-spring-boot) docker container's cold start time to **300+ ms** 
from **3000+ ms**
## Dependencies  

1. Docker CE==17.03
2. criu from  https://github.com/LiuChangFreeman/criu
3. runc from  https://github.com/LiuChangFreeman/runc
4. containerd from  https://github.com/LiuChangFreeman/containerd 

## Requirements

Currently docker-initless is developed on a normal server with regular hardware _(a 2Core-4GB Centos 7 VPS, without fast SSD)_ . One of the most important purposes of this project is to save money, so it is **low-barries**. All you need to do is to build the docker-in-docker image and give the **--privileged** to a container instance. 

## How does it work?

docker-initless works with the **criu** project, which is known as a tool which can recover a group of froozen processes on another machine.   
docker-initless uses the `post-copy restore` to make a container runable before the pages are filled into memory which is a background task. Time spent on restoring memory pages at *GB* level will be reduced significantly.     
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
