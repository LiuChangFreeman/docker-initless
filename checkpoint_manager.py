# -*- coding: utf-8 -*-
from __future__ import print_function
from utils import *
import os
import json
import shutil
import time
import uuid
import yaml


def build_image(path_sample, config):
    global log

    service_name = config["service_name"]
    image_name = config["image_name"]
    tag_name = config["tag_name"]
    image_tag = "{}:{}".format(image_name, tag_name)
    try:
        docker_client.images.get(image_tag)
        log.info("[{}] Image has been built, skip it".format(service_name))
        return 
    except:
        pass
    
    log.info("[{}] Start to build image".format(service_name))

    response = docker_api_client.build(
        path=path_sample, rm=True, tag=image_tag, forcerm=True, nocache=False
    )

    is_success = False
    for line in response:
        log_stream = json.loads(line)
        if log_stream.keys()[0] in ('stream', 'error'):
            value = log_stream.values()[0].strip()
            if value:
                try:
                    os.system("clear")
                    print(value)
                except:
                    pass
                if "Successfully built" in line:
                    is_success = True

    assert is_success


def checkpoint_container(config, settings):
    global log
    
    service_name = config["service_name"]
    image_name = config["image_name"]
    tag_name = config["tag_name"]
    checkpoint_tag_name = config["checkpoint_tag_name"]
    checkpoint_name = config["checkpoint_name"]
    service_port = config["service_port"]
    start_cmd = config["start_cmd"]
    msg_checkpoint = config["msg_checkpoint"]
    path_checkpoint_parent = settings["checkpoint_dir"]

    log.info("[{}] Start to create container".format(service_name))
    container_name = "{}-{}-{}".format(image_name, tag_name, str(uuid.uuid4()))
    command_create_checkpoint = "docker checkpoint create {} --checkpoint-dir={} {}".format(container_name,
                                                                                            path_checkpoint_parent,
                                                                                            checkpoint_name)

    path_checkpoint_current = os.path.join(path_checkpoint_parent, checkpoint_name)
    if os.path.exists(path_checkpoint_current):
        log.info("[{}] Checkpoint {} exists, remove it".format(service_name,path_checkpoint_current))
        shutil.rmtree(path_checkpoint_current)

    docker_client.containers.run(
        image="{}:{}".format(image_name, tag_name),
        command=start_cmd,
        detach=True,
        user="root",
        ports={service_port: service_port_host},
        name=container_name,
        security_opt=["seccomp=unconfined"]
    )

    try_count = 1
    while True:
        if try_count > tries_max:
            log.error("[{}] create checkpoint failed: max tries reached".format(service_name))
            break
        time.sleep(3)
        docker_logs = docker_client.containers.get(container_name).logs()
        if msg_checkpoint in docker_logs:
            log.info("[{}] Start to checkpoint".format(service_name))
            docker_client.containers.get(container_name).commit(repository=image_name, tag=checkpoint_tag_name)
            log.info("[{}] Container is committed as: {}:{}".format(service_name,image_name,checkpoint_tag_name))
            os.system(command_create_checkpoint)
            docker_client.containers.get(container_name).remove(force=True)
            log.info("[{}] Checkpoint finished".format(service_name))
            break
        else:
            try_count += 1
            log.info("[{}] Waitting for container ready".format(service_name))


def main():
    global log
    log, settings = init()
    code_dir = settings["code_dir"]
    for path_code_dir in os.listdir(code_dir):
        path_code_dir = os.path.join(code_dir, path_code_dir)
        path_dockerfile = os.path.join(path_code_dir, 'dockerfile')
        assert os.path.exists(path_dockerfile)
        path_yaml = os.path.join(path_code_dir, 'config.yaml')
        assert os.path.exists(path_yaml)
        config = yaml.load(open(path_yaml).read(),Loader=yaml.FullLoader)
        if os.path.exists(os.path.join(settings["checkpoint_dir"], config["checkpoint_name"], "config.json")):
            log.info("[checkpoint_manager] Service {} checkpoint exists, skip it".format(path_code_dir))
            continue
        log.info("[checkpoint_manager] Start to process service {}".format(path_code_dir))
        build_image(path_code_dir, config)
        checkpoint_container(config, settings)


if __name__ == "__main__":
    main()