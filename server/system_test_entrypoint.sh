#!/bin/sh

# We are restarting the exited game server in this script instead of relying 
# on the docker-compose to restart it because we want to collect the code coverage
# file between the restarts.

RESTART_DELAY_SEC=5

rm -f /app/code_coverage/*
while true; do
    /app/game-server.test --server \
        -test.coverprofile=/app/code_coverage/system_test_coverage_$(date +%s).out \
        -test.run TestRunMain \
        voyager.com/server
    echo "Game server exited with $?. Restarting in ${RESTART_DELAY_SEC} seconds."
    sleep ${RESTART_DELAY_SEC}
done
