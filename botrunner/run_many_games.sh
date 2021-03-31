#!/usr/bin/env bash

set -e

num_games=${1:-2}
export API_SERVER_URL=${API_SERVER_URL:-"http://localhost:9501"}
export NATS_URL=${NATS_URL:-'nats://localhost:4222'}
export PRINT_GAME_MSG=${PRINT_GAME_MSG:-false}
export PRINT_HAND_MSG=${PRINT_HAND_MSG:-false}
export DEV_BOT_SCRIPT=play-many-hands.yaml
export LAUNCH_INTERVAL=${LAUNCH_INTERVAL:-1}
export LOG_DIR=${LOG_DIR:-log}

trap "trap - SIGTERM && kill -- -$$" SIGINT SIGTERM EXIT

mkdir -p ${LOG_DIR}
for (( i=1; i<=${num_games}; i++ )); do
    echo ${i}
    ./botrunner --config botrunner_scripts/${DEV_BOT_SCRIPT} > ${LOG_DIR}/botrunner_${i}.out 2>&1 &
    sleep ${LAUNCH_INTERVAL}
done

echo "Launched all the bots. Ctrl+C or kill this script (pid $$) to stop them."

while true; do sleep 1000; done
