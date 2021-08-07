FROM centos:7.6.1810
ENV GITHUB_HTTP_PORXY=https://ghproxy.com

#Install dev tools
RUN curl -o /etc/yum.repos.d/CentOS-Base.repo https://mirrors.aliyun.com/repo/Centos-7.repo
RUN sed -i -e '/mirrors.cloud.aliyuncs.com/d' -e '/mirrors.aliyuncs.com/d' /etc/yum.repos.d/CentOS-Base.repo
RUN yum-config-manager --add-repo http://mirrors.aliyun.com/docker-ce/linux/centos/docker-ce.repo
RUN yum makecache
RUN yum install -y yum-utils git which make gcc unzip wget
RUN yum install -y epel-release
RUN yum install -y python python-pip

#Install docker-ce-17.03.3
RUN yum install -y https://mirrors.aliyun.com/docker-ce/linux/centos/7.6/x86_64/stable/Packages/docker-ce-selinux-17.03.3.ce-1.el7.noarch.rpm
RUN yum install -y docker-ce-17.03.3.ce

#Install criu
WORKDIR /usr/local
RUN yum install -y libseccomp-devel protobuf protobuf-c protobuf-c-devel protobuf-compiler protobuf-devel protobuf-python pkg-config python-ipaddress libbsd-devel iproute2 nftables libcap-devel libnet-devel libnl3-devel libaio-devel python2-future
RUN git clone $GITHUB_HTTP_PORXY/https://github.com/LiuChangFreeman/criu.git -b main --single-branch
WORKDIR /usr/local/criu
RUN make
RUN ln -s /usr/local/criu/criu/criu /usr/local/bin/criu
WORKDIR /home

#Install containerd
RUN wget $GITHUB_HTTP_PORXY/https://github.com/LiuChangFreeman/containerd/releases/download/latest/containerd.zip -O containerd.zip
RUN unzip containerd.zip -d /usr/local/bin/
RUN rm -rf containerd.zip
RUN mv /usr/local/bin/containerd /usr/local/bin/docker-containerd
RUN mv /usr/local/bin/containerd-shim /usr/local/bin/docker-containerd-shim
RUN mv /usr/local/bin/ctr /usr/local/bin/docker-containerd-ctr

#Install runc
RUN  wget $GITHUB_HTTP_PORXY/https://github.com/LiuChangFreeman/runc/releases/download/latest/docker-runc
RUN mv docker-runc /usr/local/bin/docker-runc

#Install docker-initless
RUN  wget $GITHUB_HTTP_PORXY/https://github.com/LiuChangFreeman/docker-initless/releases/download/latest/docker-initless
RUN mv docker-initless /usr/local/bin/docker-initless

#Other installation
RUN chmod +x /usr/local/bin/docker-*
RUN systemctl enable docker
WORKDIR /etc/docker
RUN echo "{\"experimental\":true,\"registry-mirrors\": [\"https://hub-mirror.c.163.com\"],\"bip\": \"192.168.199.1/24\"}" > daemon.json
WORKDIR /home
ADD settings.yaml /etc/docker/settings.yaml
ADD docker-initless.service /etc/systemd/system/docker-initless.service
RUN systemctl enable docker-initless
RUN pip install requests redis==3.5.3 PyYAML==5.3 websocket-client==0.32.0 dockerfile_parse pathlib==1.0.1 docker==4.4.4 -i https://mirrors.aliyun.com/pypi/simple/
RUN yum install -y e4fsprogs iptables.x86_64 net-tools