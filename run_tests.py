# encoding=utf-8
from utils import *
from redis import Redis
from multiprocessing import Process, Lock, Queue
import os
import shutil
import time
import requests
import json
import socket

instance_count=1
epoch_count=5
mutex_lock = Lock()
queue = Queue(instance_count)
settings = yaml.load(open(path_settings).read(), Loader=yaml.FullLoader)
path_checkpoint_temp=os.path.join(settings["checkpoint_dir"],"temp")

def start_lazy_page_server(path_checkpoint_current):
    os.system("criu lazy-pages --images-dir {}".format(path_checkpoint_current))

def pre_start_container(container_id,config):
    global log,mutex_lock
    image_name = config["image_name"]
    service_name=config["service_name"]
    container_name = "{}_{}".format(image_name,container_id)
    os.system("docker start --checkpoint-dir={} --checkpoint=v{}-merge {}".format(path_checkpoint_temp,container_id, container_name))
    log.info("[{}] Container started".format(service_name))
    time_boot_start=queue.get()
    time_duration=round(time.time() - time_boot_start, 6)
    log.info("[{}] Docker start time: {}s".format(service_name,time_duration))


def run_container_initless(container_id, config):
    global log,mutex_lock
    container_port = 9000 + container_id

    service_name=config["service_name"]
    image_name = config["image_name"]
    start_cmd = config["start_cmd"]
    checkpoint_tag_name = config["checkpoint_tag_name"]
    service_port = config["service_port"]
    checkpoint_name=config["checkpoint_name"]
    container_name = "{}_{}".format(image_name,container_id)
    path_checkpoint_upper = os.path.join(path_checkpoint_temp, "v{}".format(container_id))
    path_checkpoint_merge = os.path.join(path_checkpoint_temp, "v{}-merge".format(container_id))
    path_checkpoint_lower=os.path.join(settings["checkpoint_dir"],checkpoint_name)

    log.info("[{}] Prepare checkpoint imgs using overlay-fs".format(service_name))
    if os.path.exists(path_checkpoint_merge):
        os.system("umount -lf {}".format(path_checkpoint_merge))
        shutil.rmtree(path_checkpoint_merge)
    os.mkdir(path_checkpoint_merge)
    if os.path.exists(path_checkpoint_upper):
        shutil.rmtree(path_checkpoint_upper)
    os.mkdir(path_checkpoint_upper)
    command="mount -t overlay -o lowerdir={},upperdir={},workdir={} overlay {}".format(path_checkpoint_lower,path_checkpoint_upper,path_checkpoint_merge,path_checkpoint_merge)
    os.system(command)

    log.info("[{}] Create container".format(service_name))
    try:
        docker_client.containers.get(container_name).remove(force=True)
    except:
        pass

    docker_client.containers.create(
        image="{}:{}".format(image_name,checkpoint_tag_name),
        command=start_cmd,
        detach=True,
        user="root",
        ports={service_port:container_port},
        name=container_name,
        security_opt=["seccomp=unconfined"]
    )

    log.info("[{}] Start lazy-pages server".format(service_name))
    Process(target=start_lazy_page_server,args=(path_checkpoint_merge,)).start()

    log.info("[{}] Set watchdog port".format(service_name))
    container_real_id=docker_client.containers.get(container_name).id
    port_watchdog=19000+container_id
    redis_client.set(container_real_id,str(port_watchdog))

    log.info("[{}] Pre-start container".format(service_name))
    Process(target=pre_start_container,args=(container_id,config,)).start()

    log.info("[{}] Will start the container after 3s".format(service_name))
    time.sleep(3)

    time_boot_start = time.time()
    queue.put(time_boot_start)
    try:
        log.info("[{}] Try to start container".format(service_name))
        sk=socket.socket(socket.AF_INET,socket.SOCK_STREAM)
        sk.connect(("0.0.0.0",port_watchdog))
    except Exception as e:
        if e.errno!=111:
            log.error("[{}] Start container error: {}".format(service_name,str(e)))
    
    for _ in range(1):
        health_check=config["health_check"]
        while True:
            if time.time()-time_boot_start>10:
                log.error("[{}] Start container failed: max tries reached".format(service_name))
                break
            try:
                url="http://0.0.0.0:{}{}".format(container_port, health_check["path"])
                response=requests.get(url,timeout=0.1)
                assert health_check["wanted"] in response.text
                log.info("[{}] Health check want: {}".format(service_name,health_check["wanted"]))
                log.info("[{}] Got: {}, assert success".format(service_name,response.text))
                break
            except:
                pass
    time_requests_end = time.time()
    time_total_duration=round(time_requests_end - time_boot_start, 6)
    log.info("[{}] Request totally cost: {}s".format(service_name,time_total_duration))

    os.system("umount -lf {}".format(path_checkpoint_merge))
    shutil.rmtree(path_checkpoint_merge)
    shutil.rmtree(path_checkpoint_upper)
    try:
        redis_client.delete(container_real_id)
    except:
        pass
    docker_client.containers.get(container_name).remove(force=True)
    log.info("[{}] Test finished".format(service_name))

def test_initless(path_code_dir):
    global log
    path_yaml = os.path.join(path_code_dir, 'config.yaml')
    config = yaml.load(open(path_yaml).read(), Loader=yaml.FullLoader)
    if config["is_enabled"]:
        log.info("[{}] Start to test initless boot".format(config["service_name"]))
        for i in range(epoch_count):
            log.info("--------Epoch {}--------".format(i + 1))
            for container_id in range(1,1+instance_count):
                run_container_initless(container_id, config)
            log.info("--------Epoch end--------")

def run_container_normal(container_id, config):
    global log
    container_port = 9000 + container_id
    service_name=config["service_name"]
    image_name = config["image_name"]
    start_cmd = config["start_cmd"]
    checkpoint_tag_name = config["checkpoint_tag_name"]
    service_port = config["service_port"]
    container_name = "{}_{}".format(image_name,container_id)

    log.info("[{}] Run container".format(service_name))
    try:
        docker_client.containers.get(container_name).remove(force=True)
    except:
        pass

    time_boot_start = time.time()
    docker_client.containers.run(
        image="{}:{}".format(image_name,checkpoint_tag_name),
        command=start_cmd,
        detach=True,
        user="root",
        ports={service_port:container_port},
        name=container_name,
        security_opt=["seccomp=unconfined"]
    )

    for _ in range(1):
        health_check=config["health_check"]
        while True:
            if time.time()-time_boot_start>10:
                log.error("[{}] Start container failed: max tries reached".format(service_name))
                break
            try:
                url="http://0.0.0.0:{}{}".format(container_port, health_check["path"])
                response=requests.get(url,timeout=0.1)
                assert health_check["wanted"] in response.text
                log.info("[{}] Health check want: {}".format(service_name,health_check["wanted"]))
                log.info("[{}] Got: {}, assert success".format(service_name,response.text))
                break
            except:
                pass

    time_requests_end = time.time()
    time_total_duration=round(time_requests_end - time_boot_start, 6)
    log.info("[{}] Total cost: {}s".format(service_name,time_total_duration))
    docker_client.containers.get(container_name).remove(force=True)
    log.info("[{}] Test finished".format(service_name))

def test_normal(path_code_dir):
    global log
    path_yaml = os.path.join(path_code_dir, 'config.yaml')
    config = yaml.load(open(path_yaml).read(), Loader=yaml.FullLoader)
    if config["is_enabled"]:
        log.info("[{}] Start to test normal boot".format(config["service_name"]))
        for i in range(epoch_count):
            log.info("--------Epoch {}--------".format(i + 1))
            for container_id in range(1,1+instance_count):
                run_container_normal(container_id, config)
            log.info("--------Epoch end--------")

def main():
    global log,redis_client
    log,settings=init()
    redis_client=Redis(host=settings["redis_host"],port=settings["redis_port"])
    for code_dir in os.listdir(settings["code_dir"]):
        path_code_dir= os.path.join(settings["code_dir"], code_dir)
        test_initless(path_code_dir)
        test_normal(path_code_dir)

if __name__=="__main__":
    main()