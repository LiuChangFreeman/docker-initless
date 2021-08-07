systemctl restart docker
umount -lf checkpoint/temp/*
rm -rf checkpoint/temp/*
docker rm `docker ps -a | grep Created | awk '{print $1}'` || true
docker rm `docker ps -a | grep Exited | awk '{print $1}'` || true