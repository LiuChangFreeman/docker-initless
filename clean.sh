umount -lf checkpoint/temp/*
rm -rf checkpoint/temp/*
ps -ef| grep 'docker-runc'  | awk '{print $2}' |xargs kill -9
ps -ef| grep 'criu'  | awk '{print $2}' |xargs kill -9
docker rm `docker ps -a | grep Created | awk '{print $1}'` || true
docker rm `docker ps -a | grep Exited | awk '{print $1}'` || true