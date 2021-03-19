#!/usr/bin/env bash

set -e

# YAML file paths are relative to the botrunner.
test_scripts=( 
    'botrunner_scripts/river-action-3-bots.yaml'
)

API_SERVER_URL=http://api-server:9501
GAME_SERVER_URL=http://game-server:8080

for script in "${test_scripts[@]}"; do
    docker exec -t system_test_botrunner_1 bash -c "\
        while ! curl -s ${API_SERVER_URL} >/dev/null; do \
            echo 'Waiting for API server ${API_SERVER_URL}'; \
            sleep 1; \
        done \
        && \
        while ! curl -s ${GAME_SERVER_URL} >/dev/null; do \
            echo 'Waiting for game server ${GAME_SERVER_URL}'; \
            sleep 1; \
        done \
        && \
        ./botrunner --script ${script} \
    "
done
