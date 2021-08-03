final num_log_lines = 200

pipeline {
    agent any
    options {
        disableConcurrentBuilds()
    }
    stages {
        stage('env') {
            steps {
                sh 'printenv | sort'
                sh 'pwd'
                sh 'ls -l'
            }
        }
        stage('Clean Up Containers') {
            steps {
                cleanUpContainers()
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
            script {
                printLastNLines('jenkins_logs/docker_test.log', num_log_lines)
                printLastNLines('jenkins_logs/system_test.log', num_log_lines)
            }
            archiveArtifacts artifacts: 'jenkins_logs/*', allowEmptyArchive: true
            cleanUpContainers()
        }
        success {
            setBuildStatus("Build succeeded", "SUCCESS");
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
    repoUrl = getRepoURL()
    step([
        $class: "GitHubCommitStatusSetter",
        reposSource: [$class: "ManuallyEnteredRepositorySource", url: repoUrl],
        contextSource: [$class: "ManuallyEnteredCommitContextSource", context: "ci/jenkins/build-status"],
        errorHandlers: [[$class: "ChangingBuildStatusErrorHandler", result: "UNSTABLE"]],
        statusResultSource: [ $class: "ConditionalStatusResultSource", results: [[$class: "AnyBuildResult", message: message, state: state]] ]
    ]);
}

def getRepoURL() {
  sh "git config --get remote.origin.url > .git/remote-url"
  return readFile(".git/remote-url").trim()
}

/*
Clean up running containers.
Be careful not to remove the jenkins container, etc.
*/
def cleanUpContainers() {
    sh 'docker rm -f $(docker ps | grep -v "jenkins" | awk \'{print $1}\' | tail -n +2) || true'
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
