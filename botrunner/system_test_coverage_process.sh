#!/usr/bin/env bash

# Merges all coverage files in the directory $2 and
# Generates an html file in the server directory $1.
set -e

server_src_dir=$(realpath -s ${1})
coverage_file_dir=$(realpath -s ${2})

echo "Processing system test coverage files."
echo "Game serve source dir: ${server_src_dir}"
echo "Coverage files dir: ${coverage_file_dir}"

pushd ${coverage_file_dir}
ls -l
coverage_files=$(ls)
gocovmerge ${coverage_files} > system_test_coverage_merged.out
popd

pushd ${server_src_dir}
rm -f ./system_test_coverage_merged.out ./system_test_coverage_merged.html
mv ${coverage_file_dir}/system_test_coverage_merged.out ./
if ! [ -z "${CI}" ]; then
    go tool cover -html=system_test_coverage_merged.out -o system_test_coverage_merged.html
else
    go tool cover -html=system_test_coverage_merged.out
fi
popd
