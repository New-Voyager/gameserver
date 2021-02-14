#!/usr/bin/env bash

# This script is used to launch the system test in the docker container.

set -e

check_api_server() {
    http_status=$(curl -s --max-time 3 --show-error -o /dev/null -w "%{http_code}" ${API_SERVER_URL})
}

errs=0

if [ -z ${API_SERVER_URL} ]; then
    echo "API_SERVER_URL is not defined."
    ((errs++))
fi

if [ -z ${PERSIST_METHOD} ]; then
    echo "PERSIST_METHOD is not defined."
    ((errs++))
fi

if [ -z ${REDIS_HOST} ]; then
    echo "REDIS_HOST is not defined."
    ((errs++))
fi

if [ -z ${REDIS_PORT} ]; then
    echo "REDIS_PORT is not defined."
    ((errs++))
fi

if [ -z ${REDIS_DB} ]; then
    echo "REDIS_DB is not defined."
    ((errs++))
fi

if [ ${errs} -ne 0 ]; then
    exit 1
fi

# Wait for the api server to be ready.
MAX_WAIT=${MAX_WAIT:-30}
remaining=${MAX_WAIT}
until curl -m 2 ${API_SERVER_URL}; do
    ((remaining--))
    if [ ${remaining} -le 0 ]; then
        echo "API server not responded in ${MAX_WAIT} attempts"
        exit 1
    fi
    echo "Waiting for api server (${remaining})"
    sleep 1
done;

./test --config config.yaml --game-server-dir=../server --botrunner-dir=../botrunner
