# -*- coding: utf-8 -*-
from __future__ import print_function
from logging import handlers
from docker import APIClient
import os
import logging
import docker
import yaml

docker_client = docker.from_env()
docker_api_client = APIClient(base_url='unix://var/run/docker.sock')
path_current = os.path.split(os.path.realpath(__file__))[0]
path_settings="/etc/docker/settings.yaml"
path_logs= os.path.join(path_current, 'logs')
service_port_host = 9001
tries_max = 10

def init():
    if not os.path.exists(path_logs):
        os.mkdir(path_logs)
    log = logging.getLogger()
    log.setLevel(logging.DEBUG)
    fmt = logging.Formatter(fmt="%(asctime)s[]%(message)s", datefmt='%D %H:%M:%S')
    terminal_handler = logging.StreamHandler()
    terminal_handler.setLevel(logging.DEBUG)
    terminal_handler.setFormatter(fmt)
    path_file = os.path.join(path_current, path_logs, 'log.log')
    file_handler = handlers.TimedRotatingFileHandler(filename=path_file, when='D', encoding="utf-8", backupCount=7)
    file_handler.setLevel(logging.DEBUG)
    file_handler.setFormatter(fmt)
    log.addHandler(terminal_handler)
    log.addHandler(file_handler)
    settings = yaml.load(open(path_settings).read())

    path_checkpoint_parent= settings["checkpoint_dir"]
    path_checkpoint_temp=os.path.join(path_checkpoint_parent,"temp")
    if not os.path.exists(path_checkpoint_parent):
        os.mkdir(path_checkpoint_parent)
    if not os.path.exists(path_checkpoint_temp):
        os.mkdir(path_checkpoint_temp)

    return log,settings