#!/usr/bin/env bash

# export API_SERVER_URL='http://localhost:9501'

export BOT_SCRIPT='botrunner_scripts/river-action-3-bots.yaml'

docker exec -t system_test_botrunner_1 \
    ./botrunner \
    --script ${BOT_SCRIPT}
