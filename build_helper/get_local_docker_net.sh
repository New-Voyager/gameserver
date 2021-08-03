set -e

if ! [ -z ${BUILD_ID} ]; then
    echo $1__${BUILD_ID}
else
    echo $1
fi
