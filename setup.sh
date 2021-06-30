yum install -y yum-utils device-mapper-persistent-data lvm2
yum-config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo
yum makecache
yum install -y docker-ce-17.03.3.ce
systemctl enable docker
systemctl start docker
yum install -y git make gcc libseccomp-devel protobuf protobuf-c protobuf-c-devel protobuf-compiler protobuf-devel protobuf-python pkg-config python-ipaddress libbsd-devel iproute2 nftables libcap-devel libnet-devel libnl3-devel libaio-devel python2-future
git clone https://github.com/LiuChangFreeman/criu.git -b mod --single-branch
cd criu
make
ln -s $(pwd)/criu/criu /usr/local/bin/criu
git clone https://github.com/LiuChangFreeman/runc.git -b main --single-branch
cd runc
make
ln -s $(pwd)/runc /usr/local/bin/docker-runc