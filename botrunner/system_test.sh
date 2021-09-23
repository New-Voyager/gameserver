#!/usr/bin/env bash

set -e

# Kill the script after this many seconds. Scripts that need additional time should add
# the following line inside the script yaml file.
#
# # SYSTEM_TEST_TIMEOUT_SEC:180
#
DEFAULT_TIMEOUT_SEC=${DEFAULT_TIMEOUT_SEC:-60}

serial_scripts=(
    $(find . -path './botrunner_scripts/system_test/serial/*.yaml')
)

parallel_scripts=(
    $(find . -path './botrunner_scripts/system_test/parallel/*.yaml')
)

# serial_scripts=(
#     './botrunner_scripts/system_test/locationcheck/ipcheck/sitback.yaml'
#     './botrunner_scripts/system_test/locationcheck/gpscheck/sitback.yaml'
# )
# parallel_scripts=()

all_scripts=( "${serial_scripts[@]}" "${parallel_scripts[@]}" )

echo "DEFAULT_TIMEOUT_SEC: ${DEFAULT_TIMEOUT_SEC}"
echo "Test Scripts: ${all_scripts[@]}"

succeeded_scripts=succeeded.txt
failed_scripts=failed.txt
timedout_scripts=timedout.txt
> ${succeeded_scripts}
> ${failed_scripts}
> ${timedout_scripts}

runscript() {
    local script=${1}
    local timeout_sec=${DEFAULT_TIMEOUT_SEC}
    local custom_timeout_sec=$(grep '# SYSTEM_TEST_TIMEOUT_SEC:' "${script}" | head -1 | cut -d: -f2)
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

    local exit_code=0

    local start_sec=${SECONDS}
    # We're wrapping the test with timeout command to help us manage the run time.
    # 'timeout N' forces the botrunner to exit after the specified seconds and
    # is used to prevent scripts hanging forever during CI.
    timeout ${timeout_sec} ./botrunner --script ${script} || exit_code=$?
    local end_sec=${SECONDS}
    local run_sec=$((end_sec - start_sec))
    if [ $exit_code -eq 0 ]; then
        echo -e "\nPASS: ${script}\n\n"
        echo "- ${script} (${run_sec}s)" >> ${succeeded_scripts}
    else
        # Timeout command should return 124 on timeout, but seems to return 143 on the botrunner alpine.
        if [ $exit_code -eq 124 ] || [ $exit_code -eq 143 ]; then
            echo -e "\nTIMEOUT (${timeout_sec}s): ${script}\n\n"
            echo "- ${script} (${timeout_sec}s)" >> ${timedout_scripts}
        else
            echo -e "\nFAIL: ${script}\n\n"
            echo "- ${script} (${run_sec}s)" >> ${failed_scripts}
        fi
    fi
}

# Run the scripts that need to be run one at a time.
for script in "${serial_scripts[@]}"; do
    runscript $script
done

# Run the remaining scripts in parallel. Capture the output to files.
rm -rf test_log
mkdir test_log
for script in "${parallel_scripts[@]}"; do
    echo "Launching ${script}"
    ( runscript $script ) > test_log/$(basename ${script}).log 2>&1 &
done

echo
echo "Waiting for tests to finish"
wait

# Make the game server exit and create the code coverage file.
curl -X POST ${GAME_SERVER_1_URL}/end-system-test
curl -X POST ${GAME_SERVER_2_URL}/end-system-test
sleep 2

# Print the captured output files.
log_files=(
    $(find . -path './test_log/*')
)
for log_file in "${log_files[@]}"; do
    cat ${log_file}
done

num_failed=$(wc -l < ${failed_scripts})
num_timedout=$(wc -l < ${timedout_scripts})

echo
echo
echo "################################################################################"
echo
echo "RESULT"
echo
echo "Succeeded ($(wc -l < ${succeeded_scripts})):"
cat ${succeeded_scripts}
echo
echo "Failed (${num_failed}):"
cat ${failed_scripts}
echo
echo "Timed Out (${num_timedout}):"
cat ${timedout_scripts}
echo
echo "################################################################################"

if [ "${num_failed}" -ne 0 ] || [ "${num_timedout}" -ne 0 ]; then
    exit 1
fi
