docker build -t docker-initless:dind .
mkdir -p /var/lib/docker-initless
docker run --name docker-initless -d --privileged --net=host --restart=always -v $(pwd):/home -v /var/lib/docker-initless:/var/lib/docker -v /tmp:/tmp -v /lib/modules/:/lib/modules/ docker-initless:dind /usr/sbin/init
docker exec -it  docker-initless /bin/bash  
docker run -p 0.0.0.0:6380:6379 --name redis --restart=always -d redis redis-server