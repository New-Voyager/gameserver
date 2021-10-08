final num_log_lines = 200

pipeline {
    agent any
    options {
        // No concurrent builds within branch.
        disableConcurrentBuilds()
        // The build time currently includes the time waiting for an available executor,
        // so we need to give it some extra time here.
        timeout(time: 120, unit: 'MINUTES')
    }
    environment {
        DOCKER_TEST_LOG = "docker_test.log"
        SYSTEM_TEST_LOG = "system_test.log"
    }
    stages {
        stage('Setup') {
            steps {
                setBuildStatus("Build is in progress", "PENDING");
                sh 'printenv | sort'
                sh 'pwd'
                sh 'ls -l'
            }
        }
        stage('Build') {
            steps {
                sh 'make docker-build'
            }
        }
        stage('Unit Test') {
            steps {
                sh 'make test'
            }
        }
        stage('Docker Test') {
            steps {
                sh 'rm -rf jenkins_logs && mkdir jenkins_logs'
                echo "Running Docker Test. Last ${num_log_lines} lines of the log will be printed at the end and the full log will be saved as a build artifact (${DOCKER_TEST_LOG})."
                script {
                    try {
                        sh "make docker-test > jenkins_logs/${DOCKER_TEST_LOG} 2>&1"
                    } finally {
                        printLastNLines("jenkins_logs/${DOCKER_TEST_LOG}", num_log_lines)
                    }
                }
            }
        }
        stage('System Test') {
            steps {
                echo "Running System Test. Last ${num_log_lines} lines of the log will be printed at the end and the full log will be saved as a build artifact (${SYSTEM_TEST_LOG})."
                script {
                    try {
                        sh "make system-test > jenkins_logs/${SYSTEM_TEST_LOG} 2>&1"
                    } finally {
                        printLastNLines("jenkins_logs/${SYSTEM_TEST_LOG}", num_log_lines)
                        sh "SVC=api-server make print-system-test-logs > jenkins_logs/api-server.log"
                        sh "SVC=game-server-1 make print-system-test-logs > jenkins_logs/game-server-1.log"
                        sh "SVC=game-server-2 make print-system-test-logs > jenkins_logs/game-server-2.log"
                        sh "cat jenkins_logs/game-server-1.log > jenkins_logs/game-server-both.log"
                        sh "cat jenkins_logs/game-server-2.log >> jenkins_logs/game-server-both.log"
                        sh "SVC=timer make print-system-test-logs > jenkins_logs/timer.log"
                        sh "SVC=scheduler make print-system-test-logs > jenkins_logs/scheduler.log"
                    }
                    sh "mv server/system_test_coverage_merged.html jenkins_logs/"
                }
            }
        }
        stage('Publish') {
            when {
                expression { return env.GIT_BRANCH == 'master' }
            }
            steps {
                sh 'make publish'
            }
        }
    }
    post {
        always {
            archiveArtifacts artifacts: 'jenkins_logs/*', allowEmptyArchive: true
            cleanUpDockerResources()
            cleanUpBuild()
        }
        success {
            setBuildStatus("Build succeeded", "SUCCESS");
        }
        aborted {
            setBuildStatus("Build aborted", "FAILURE");
        }
        failure {
            setBuildStatus("Build failed", "FAILURE");
        }
    }
}

/*
Notify GitHub the build result.
https://plugins.jenkins.io/github/
*/
def setBuildStatus(String message, String state) {
    step([
        $class: "GitHubCommitStatusSetter",
        reposSource: [$class: "ManuallyEnteredRepositorySource", url: env.GIT_URL],
        contextSource: [$class: "ManuallyEnteredCommitContextSource", context: "ci/jenkins"],
        errorHandlers: [[$class: "ChangingBuildStatusErrorHandler", result: "UNSTABLE"]],
        statusResultSource: [ $class: "ConditionalStatusResultSource", results: [[$class: "AnyBuildResult", message: message, state: state]] ]
    ]);
}

def cleanUpBuild() {
    sh 'make clean-ci'
}

/*
Clean up docker images and other docker resources.
*/
def cleanUpDockerResources() {
    // Remove old stopped containers.
    sh 'docker container prune --force --filter until=12h || true'
    // Remove dangling images.
    sh 'docker image prune --force || true'
    // Remove old unused networks. Being a little aggressive here due to limited IP address pool.
    sh 'docker network prune --force --filter until=120m || true'
    // Remove unused volumes.
    sh 'docker volume prune --force || true'
}

/*
Print last n lines of a text file. Call this inside a script block.

script {
    printLastNLines('jenkins_logs/docker_test.log', 200)
}
*/
def printLastNLines(String file, int numLines) {
    if (fileExists(file)) {
        echo """###################################################
                |Last ${numLines} lines of ${file}
                |###################################################
                """.stripMargin().stripIndent()
        sh "tail -n ${numLines} ${file}"
    }
}
