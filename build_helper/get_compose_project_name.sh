set -e

if ! [ -z ${CI} ]; then
    branch_suffix=$(echo ${GIT_BRANCH} | sed 's/\//_/g' | sed 's/-/_/g')
    echo $1__${branch_suffix}_${BUILD_ID}
else
    echo $1
fi
