#!/usr/bin/env bash

set -e

# YAML file paths are relative to the botrunner.
# 'botrunner_scripts/river-action-3-bots.yaml'
test_scripts=(
    'botrunner_scripts/river-action-3-bots.yaml'
    'botrunner_scripts/crash_test/river-action-3-bots-wait-for-next-action.yaml'
    'botrunner_scripts/crash_test/river-action-3-bots-move-to-next-action.yaml'
)

for script in "${test_scripts[@]}"; do
    docker exec -t system_test_botrunner_1 bash -c "\
        while ! curl -s \${API_SERVER_URL} >/dev/null; do \
            echo Waiting for API server \${API_SERVER_URL}; \
            sleep 1; \
        done \
        && \
        while ! curl -s \${GAME_SERVER_URL} >/dev/null; do \
            echo Waiting for game server \${GAME_SERVER_URL}; \
            sleep 1; \
        done \
        && \
        ./botrunner --script ${script} \
    "
done
