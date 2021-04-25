#!/usr/bin/env bash

set -e

# YAML file paths are relative to the botrunner.
test_scripts=(
    'botrunner_scripts/system_test/river-action-3-bots.yaml'
    'botrunner_scripts/system_test/river-action-3-bots-wait-for-next-action.yaml'
    'botrunner_scripts/system_test/river-action-3-bots-move-to-next-action.yaml'
    'botrunner_scripts/system_test/river-action-3-bots-move-to-next-round.yaml'
)

# Generate additional scripts based on templates.
generated_dir='botrunner_scripts/system_test/generated/'
mkdir -p "${generated_dir}"
for seq in {1..8}; do
    out_file="${generated_dir}/3-bots-all-players-all-in-${seq}.yaml"
    sed "s/{{SEQ}}/${seq}/g" 'botrunner_scripts/system_test/template/3-bots-all-players-all-in-template.yaml' > ${out_file} && \
    test_scripts+=(${out_file})
done

echo "Test Scripts: ${test_scripts[@]}"

for script in "${test_scripts[@]}"; do
    while ! curl -s ${API_SERVER_URL} >/dev/null; do
        echo Waiting for API server ${API_SERVER_URL}
        sleep 1
    done
    while ! curl -s ${GAME_SERVER_URL} >/dev/null; do
        echo Waiting for game server ${GAME_SERVER_URL}
        sleep 1
    done
    exit_code=0
    ./botrunner --script ${script} || exit_code=$?
    if [ $exit_code -ne 0 ]; then
        echo "System test failed on script ${script}"
        exit $exit_code
    fi
done

echo "Finished system test. Tested Scripts: ${test_scripts[@]}"
