ifdef JENKINS_VERSION
DOCKER_IMAGE_TAG := $(subst $(_slash),$(_underscore),$(GIT_BRANCH))__$(BUILD_ID)
endif