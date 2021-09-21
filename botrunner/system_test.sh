#!/usr/bin/env bash

set -e

# Abort immediately when the first script fails without running the remaining scripts.
ABORT_ON_FAIL=${ABORT_ON_FAIL:-0}

# Kill the script after this many seconds. Scripts that need additional time should add
# the following line inside the script yaml file.
#
# # SYSTEM_TEST_TIMEOUT_SEC:180
#
DEFAULT_TIMEOUT_SEC=${DEFAULT_TIMEOUT_SEC:-60}

tested_scripts=(
    $(find . -path './botrunner_scripts/system_test/*.yaml')
)
# tested_scripts=(
#     './botrunner_scripts/system_test/basic/river-action-3-bots.yaml'
#     './botrunner_scripts/system_test/timeout/timeout.yaml'
#     './botrunner_scripts/system_test/timeout/consecutive-timeout.yaml'
# )

echo "ABORT_ON_FAIL: ${ABORT_ON_FAIL}"
echo "DEFAULT_TIMEOUT_SEC: ${DEFAULT_TIMEOUT_SEC}"
echo "Test Scripts: ${tested_scripts[@]}"

succeeded_scripts=()
failed_scripts=()
timedout_scripts=()

for script in "${tested_scripts[@]}"; do
    timeout_sec=${DEFAULT_TIMEOUT_SEC}
    custom_timeout_sec=$(grep '# SYSTEM_TEST_TIMEOUT_SEC:' "${script}" | head -1 | cut -d: -f2)
    if ! [ -z ${custom_timeout_sec} ]; then
        timeout_sec=${custom_timeout_sec}
    fi

    echo -e "\n\n\n\n################################################################################"
    echo "Next Script  : ${script}"
    echo "Allowed Time : ${timeout_sec}s"
    echo -e "################################################################################\n\n"
    while ! curl -s ${API_SERVER_URL} >/dev/null; do
        echo Waiting for API server ${API_SERVER_URL}
        sleep 1
    done
    while ! curl -s ${GAME_SERVER_1_URL} >/dev/null; do
        echo Waiting for game server ${GAME_SERVER_1_URL}
        sleep 1
    done
    while ! curl -s ${GAME_SERVER_2_URL} >/dev/null; do
        echo Waiting for game server ${GAME_SERVER_2_URL}
        sleep 1
    done

    exit_code=0

    start_sec=${SECONDS}
    # We're wrapping the test with timeout command to help us manage the run time.
    # 'timeout N' forces the botrunner to exit after the specified seconds and
    # is used to prevent scripts hanging forever during CI.
    timeout ${timeout_sec} ./botrunner --script ${script} || exit_code=$?
    end_sec=${SECONDS}
    run_sec=$((end_sec - start_sec))
    if [ $exit_code -eq 0 ]; then
        echo -e "\n\nPASS: ${script}"
        succeeded_scripts+=("${script} (${run_sec}s)")
    else
        # Timeout command should return 124 on timeout, but seems to return 143 on the botrunner alpine.
        if [ $exit_code -eq 124 ] || [ $exit_code -eq 143 ]; then
            echo -e "\n\nTIMEOUT (${timeout_sec}s): ${script}"
            timedout_scripts+=("${script} (${timeout_sec}s)")
        else
            echo -e "\n\nFAIL: ${script}"
            failed_scripts+=("${script} (${run_sec}s)")
        fi
        if [ "${ABORT_ON_FAIL}" = "1" ]; then
            exit $exit_code
        fi
    fi
done

# Make the game server exit and create the code coverage file.
curl -X POST ${GAME_SERVER_1_URL}/end-system-test
curl -X POST ${GAME_SERVER_2_URL}/end-system-test
sleep 2

echo
echo
echo "################################################################################"
echo
echo "RESULT"
echo
echo "Tested (${#tested_scripts[@]}):"
printf -- '- %s\n' "${tested_scripts[@]}"
echo
echo "Succeeded (${#succeeded_scripts[@]}):"
printf -- '- %s\n' "${succeeded_scripts[@]}"
echo
echo "Failed (${#failed_scripts[@]}):"
printf -- '- %s\n' "${failed_scripts[@]}"
echo
echo "Timed Out (${#timedout_scripts[@]}):"
printf -- '- %s\n' "${timedout_scripts[@]}"
echo
echo "################################################################################"

if [ "${#succeeded_scripts[@]}" -ne "${#tested_scripts[@]}" ]; then
    exit 1
fi
