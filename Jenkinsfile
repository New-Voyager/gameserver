final num_log_lines = 200

pipeline {
    agent any
    options {
        disableConcurrentBuilds()
        timeout(time: 30, unit: 'MINUTES')
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
                sh 'mkdir -p jenkins_logs'
                echo "Running Docker Test. Last ${num_log_lines} lines of the log will be printed at the end and the full log will be saved as an artifact (docker_test.log)."
                sh 'make docker-test > jenkins_logs/docker_test.log 2>&1'
            }
        }
        stage('System Test') {
            steps {
                echo "Running System Test. Last ${num_log_lines} lines of the log will be printed at the end and the full log will be saved as an artifact (system_test.log)."
                sh 'make system-test > jenkins_logs/system_test.log 2>&1'
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
            script {
                printLastNLines('jenkins_logs/docker_test.log', num_log_lines)
                printLastNLines('jenkins_logs/system_test.log', num_log_lines)
            }
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
    sh 'docker container prune --force --filter until=12h'
    // Remove dangling images.
    sh 'docker image prune --force || true'
    // Remove old unused images.
    sh 'docker image prune -a --force --filter until=72h || true'
    // Remove old unused networks.
    sh 'docker network prune --force --filter until=72h || true'
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
