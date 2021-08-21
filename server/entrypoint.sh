#!/bin/sh

RESTART_DELAY_SEC=2

while true; do
    /app/game-server --server "$@"
    echo "Game server exited with $?. Restarting in ${RESTART_DELAY_SEC} seconds."
    sleep ${RESTART_DELAY_SEC}
done
